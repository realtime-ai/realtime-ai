package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/realtime-ai/realtime-ai/pkg/connection"
	pb "github.com/realtime-ai/realtime-ai/pkg/proto/streamingai/v1"
)

// GRPCServerConfig holds the configuration for GRPCServer
type GRPCServerConfig struct {
	Port int
}

// GRPCServer implements the StreamingAIService gRPC service
type GRPCServer struct {
	pb.UnimplementedStreamingAIServiceServer
	sync.RWMutex

	config *GRPCServerConfig

	// Active connections
	connections map[string]connection.Connection

	// gRPC server instance
	grpcServer *grpc.Server

	// Callbacks
	onConnectionCreated func(ctx context.Context, conn connection.Connection)
	onConnectionError   func(ctx context.Context, conn connection.Connection, err error)

	// Session management
	sessions map[string]*sessionInfo
}

type sessionInfo struct {
	sessionID string
	config    map[string]string
}

// NewGRPCServer creates a new gRPC server instance
func NewGRPCServer(cfg *GRPCServerConfig) *GRPCServer {
	if cfg == nil {
		cfg = &GRPCServerConfig{
			Port: 50051,
		}
	}

	return &GRPCServer{
		config:      cfg,
		connections: make(map[string]connection.Connection),
		sessions:    make(map[string]*sessionInfo),
		onConnectionCreated: func(ctx context.Context, conn connection.Connection) {
			log.Printf("[GRPCServer] Connection created (default handler): %s", conn.PeerID())
		},
		onConnectionError: func(ctx context.Context, conn connection.Connection, err error) {
			log.Printf("[GRPCServer] Connection error (default handler): %s, error: %v", conn.PeerID(), err)
		},
	}
}

// OnConnectionCreated registers a callback for when a new connection is created.
func (s *GRPCServer) OnConnectionCreated(f func(ctx context.Context, conn connection.Connection)) {
	s.onConnectionCreated = f
}

// OnConnectionError registers a callback for connection errors.
func (s *GRPCServer) OnConnectionError(f func(ctx context.Context, conn connection.Connection, err error)) {
	s.onConnectionError = f
}

// Start starts the gRPC server
func (s *GRPCServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	s.grpcServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(16 * 1024 * 1024), // 16MB
		grpc.MaxSendMsgSize(16 * 1024 * 1024), // 16MB
	)

	pb.RegisterStreamingAIServiceServer(s.grpcServer, s)

	log.Printf("[GRPCServer] Starting gRPC server on port %d", s.config.Port)

	return s.grpcServer.Serve(lis)
}

// Stop stops the gRPC server gracefully
func (s *GRPCServer) Stop() {
	if s.grpcServer != nil {
		log.Println("[GRPCServer] Stopping gRPC server...")
		s.grpcServer.GracefulStop()

		// Close all connections
		s.Lock()
		for id, conn := range s.connections {
			log.Printf("[GRPCServer] Closing connection: %s", id)
			conn.Close()
		}
		s.connections = make(map[string]connection.Connection)
		s.Unlock()
	}
}

// BiDirectionalStreaming implements the bidirectional streaming RPC
func (s *GRPCServer) BiDirectionalStreaming(
	stream pb.StreamingAIService_BiDirectionalStreamingServer,
) error {
	ctx := stream.Context()

	// Generate a unique peer ID
	peerID := uuid.New().String()
	log.Printf("[GRPCServer] New streaming connection request: %s", peerID)

	// Create gRPC connection
	grpcConn := connection.NewGRPCConnection(peerID, stream)

	// Store connection
	s.Lock()
	s.connections[peerID] = grpcConn
	s.Unlock()

	// Notify that connection is created
	s.onConnectionCreated(ctx, grpcConn)

	// Wait for the stream to end or context to be cancelled
	<-ctx.Done()

	// Clean up
	s.Lock()
	delete(s.connections, peerID)
	s.Unlock()

	log.Printf("[GRPCServer] Stream ended for peer: %s, reason: %v", peerID, ctx.Err())

	// Close connection
	if err := grpcConn.Close(); err != nil {
		s.onConnectionError(ctx, grpcConn, err)
		return status.Errorf(codes.Internal, "failed to close connection: %v", err)
	}

	return ctx.Err()
}

// CreateSession creates a new session (optional, for session management)
func (s *GRPCServer) CreateSession(
	ctx context.Context,
	req *pb.CreateSessionRequest,
) (*pb.CreateSessionResponse, error) {
	sessionID := uuid.New().String()

	s.Lock()
	s.sessions[sessionID] = &sessionInfo{
		sessionID: sessionID,
		config:    req.Config,
	}
	s.Unlock()

	log.Printf("[GRPCServer] Session created: %s", sessionID)

	return &pb.CreateSessionResponse{
		SessionId: sessionID,
	}, nil
}

// CloseSession closes an existing session
func (s *GRPCServer) CloseSession(
	ctx context.Context,
	req *pb.CloseSessionRequest,
) (*pb.CloseSessionResponse, error) {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.sessions[req.SessionId]; !exists {
		return &pb.CloseSessionResponse{
			Success: false,
		}, status.Errorf(codes.NotFound, "session not found: %s", req.SessionId)
	}

	delete(s.sessions, req.SessionId)

	log.Printf("[GRPCServer] Session closed: %s", req.SessionId)

	return &pb.CloseSessionResponse{
		Success: true,
	}, nil
}

// GetConnection returns a connection by peer ID (utility method).
func (s *GRPCServer) GetConnection(peerID string) (connection.Connection, bool) {
	s.RLock()
	defer s.RUnlock()
	conn, exists := s.connections[peerID]
	return conn, exists
}

// GetConnectionCount returns the number of active connections
func (s *GRPCServer) GetConnectionCount() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.connections)
}
