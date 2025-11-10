package kvserver

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	pb "goDistributedSystemDemo/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	PingInterval = 500 * time.Millisecond // Ping viewservice every 0.5 seconds
)

// KVServer is a key-value server that can act as Primary or Backup
type KVServer struct {
	pb.UnimplementedKVServerServer
	mu         sync.Mutex
	listener   net.Listener
	grpcServer *grpc.Server
	dead       bool
	me         string // my server name/address

	vsAddress string // view service address
	vsClient  pb.ViewServiceClient
	vsConn    *grpc.ClientConn

	currentView  *pb.View
	data         map[string]string
	role         string           // "primary", "backup", or "default"
	lastBackup   string           // last known backup address
	syncing      bool             // true when state transfer is in progress
	pendingQueue []*pb.PutRequest // queue for puts during state transfer
}

// StartServer creates and starts a new KV server
func StartServer(serverName string, vsAddress string) *KVServer {
	kv := &KVServer{
		me:           serverName,
		vsAddress:    vsAddress,
		data:         make(map[string]string),
		role:         "default",
		lastBackup:   "",
		syncing:      false,
		pendingQueue: make([]*pb.PutRequest, 0),
		currentView:  &pb.View{},
	}

	// Start listening
	lis, err := net.Listen("tcp", serverName)
	if err != nil {
		log.Fatalf("KVServer failed to listen: %v", err)
	}
	kv.listener = lis

	// Create gRPC server
	kv.grpcServer = grpc.NewServer()
	pb.RegisterKVServerServer(kv.grpcServer, kv)

	// Start gRPC server in background
	go func() {
		if err := kv.grpcServer.Serve(lis); err != nil && !kv.dead {
			log.Fatalf("KVServer failed to serve: %v", err)
		}
	}()

	// Connect to view service
	go kv.connectToViewService()

	// Start pinging view service
	go kv.pingLoop()

	log.Printf("KVServer %s started\n", serverName)
	log.Printf("KVServer Configuration: PingInterval=%v\n", PingInterval)
	return kv
}

// connectToViewService establishes connection to view service
func (kv *KVServer) connectToViewService() {
	for !kv.dead {
		conn, err := grpc.Dial(kv.vsAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			kv.mu.Lock()
			kv.vsConn = conn
			kv.vsClient = pb.NewViewServiceClient(conn)
			kv.mu.Unlock()
			log.Printf("Connected to view service at %s\n", kv.vsAddress)
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// pingLoop periodically pings the view service
func (kv *KVServer) pingLoop() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for !kv.dead {
		<-ticker.C
		kv.ping()
	}
}

// ping sends a ping to the view service and updates the view
func (kv *KVServer) ping() {
	kv.mu.Lock()
	if kv.vsClient == nil {
		kv.mu.Unlock()
		return
	}

	req := &pb.PingRequest{
		ServerName: kv.me,
		ViewNumber: kv.currentView.ViewNumber,
	}
	client := kv.vsClient
	kv.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Ping(ctx, req)
	if err != nil {
		log.Printf("Ping error: %v\n", err)
		return
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	oldView := kv.currentView
	kv.currentView = resp.View

	// Check if view has changed
	if oldView.ViewNumber != kv.currentView.ViewNumber {
		kv.handleViewChange(oldView)
	}
}

// handleViewChange handles changes in the view
func (kv *KVServer) handleViewChange(oldView *pb.View) {
	log.Printf("View changed from %d to %d (Primary: %s, Backup: %s)\n",
		oldView.ViewNumber, kv.currentView.ViewNumber,
		kv.currentView.Primary, kv.currentView.Backup)

	oldRole := kv.role

	// Determine new role
	if kv.currentView.Primary == kv.me {
		kv.role = "primary"
	} else if kv.currentView.Backup == kv.me {
		kv.role = "backup"
	} else {
		kv.role = "default"
	}

	if oldRole != kv.role {
		log.Printf("Role changed from %s to %s\n", oldRole, kv.role)
	}

	// If I became primary or if backup changed, handle state transfer
	if kv.role == "primary" {
		// Check if backup changed
		if kv.currentView.Backup != "" && kv.currentView.Backup != kv.lastBackup {
			log.Printf("New backup detected: %s, initiating state transfer\n", kv.currentView.Backup)
			kv.lastBackup = kv.currentView.Backup
			go kv.transferState(kv.currentView.Backup, kv.currentView.ViewNumber)
		} else if kv.currentView.Backup == "" {
			kv.lastBackup = ""
		}
	}
}

// transferState transfers the entire state to the new backup
func (kv *KVServer) transferState(backup string, viewNumber uint64) {
	kv.mu.Lock()
	kv.syncing = true
	dataCopy := make(map[string]string)
	for k, v := range kv.data {
		dataCopy[k] = v
	}
	kv.mu.Unlock()

	log.Printf("Transferring state to backup %s (view %d)\n", backup, viewNumber)

	// Connect to backup
	conn, err := grpc.Dial(backup, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Failed to connect to backup %s: %v\n", backup, err)
		kv.mu.Lock()
		kv.syncing = false
		kv.mu.Unlock()
		return
	}
	defer conn.Close()

	client := pb.NewKVServerClient(conn)

	// Send state
	req := &pb.SyncStateRequest{
		Data:       dataCopy,
		ViewNumber: viewNumber,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.SyncState(ctx, req)
	if err != nil {
		log.Printf("SyncState RPC failed: %v\n", err)
		kv.mu.Lock()
		kv.syncing = false
		kv.mu.Unlock()
		return
	}

	log.Printf("State transfer completed successfully\n")

	kv.mu.Lock()
	kv.syncing = false

	// Process pending puts
	if len(kv.pendingQueue) > 0 {
		log.Printf("Processing %d pending puts\n", len(kv.pendingQueue))
		pending := kv.pendingQueue
		kv.pendingQueue = make([]*pb.PutRequest, 0)
		kv.mu.Unlock()

		for _, putReq := range pending {
			kv.Put(context.Background(), putReq)
		}
	} else {
		kv.mu.Unlock()
	}
}

// Get RPC handler
func (kv *KVServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if kv.role != "primary" {
		return &pb.GetResponse{
			Value: "",
			Ok:    false,
			Error: "ErrNotPrimary",
		}, nil
	}

	value, ok := kv.data[req.Key]
	if ok {
		return &pb.GetResponse{
			Value: value,
			Ok:    true,
			Error: "",
		}, nil
	}

	return &pb.GetResponse{
		Value: "",
		Ok:    false,
		Error: "ErrNoKey",
	}, nil
}

// Put RPC handler
func (kv *KVServer) Put(ctx context.Context, req *pb.PutRequest) (*pb.PutResponse, error) {
	kv.mu.Lock()

	if kv.role != "primary" {
		kv.mu.Unlock()
		return &pb.PutResponse{
			Ok:    false,
			Error: "ErrNotPrimary",
		}, nil
	}

	// If state transfer is in progress, queue the request
	if kv.syncing {
		kv.pendingQueue = append(kv.pendingQueue, req)
		kv.mu.Unlock()
		return &pb.PutResponse{
			Ok:    true,
			Error: "",
		}, nil
	}

	backup := kv.currentView.Backup
	kv.mu.Unlock()

	// If there's a backup, forward the update
	if backup != "" {
		conn, err := grpc.Dial(backup, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Failed to connect to backup %s: %v\n", backup, err)
			// Continue anyway, update local state
		} else {
			defer conn.Close()
			client := pb.NewKVServerClient(conn)

			forwardReq := &pb.ForwardUpdateRequest{
				Key:   req.Key,
				Value: req.Value,
			}

			forwardCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			_, err = client.ForwardUpdate(forwardCtx, forwardReq)
			if err != nil {
				log.Printf("ForwardUpdate RPC failed: %v\n", err)
				// Continue anyway, update local state
			}
		}
	}

	// Update local state
	kv.mu.Lock()
	kv.data[req.Key] = req.Value
	kv.mu.Unlock()

	return &pb.PutResponse{
		Ok:    true,
		Error: "",
	}, nil
}

// ForwardUpdate RPC handler (called by Primary on Backup)
func (kv *KVServer) ForwardUpdate(ctx context.Context, req *pb.ForwardUpdateRequest) (*pb.ForwardUpdateResponse, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if kv.role != "backup" {
		return &pb.ForwardUpdateResponse{
			Ok: false,
		}, nil
	}

	kv.data[req.Key] = req.Value
	return &pb.ForwardUpdateResponse{
		Ok: true,
	}, nil
}

// SyncState RPC handler (called by Primary on new Backup for state transfer)
func (kv *KVServer) SyncState(ctx context.Context, req *pb.SyncStateRequest) (*pb.SyncStateResponse, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	log.Printf("Receiving state transfer: %d keys\n", len(req.Data))

	// Overwrite local state
	kv.data = make(map[string]string)
	for k, v := range req.Data {
		kv.data[k] = v
	}

	return &pb.SyncStateResponse{
		Ok: true,
	}, nil
}

// Kill shuts down the server
func (kv *KVServer) Kill() {
	kv.dead = true
	if kv.grpcServer != nil {
		kv.grpcServer.GracefulStop()
	}
	if kv.listener != nil {
		kv.listener.Close()
	}
	if kv.vsConn != nil {
		kv.vsConn.Close()
	}
}
