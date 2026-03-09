package grpc

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func NewServer(opts ...grpc.ServerOption) *grpc.Server {
	s := grpc.NewServer(opts...)
	reflection.Register(s)
	return s
}

func ListenAndServe(s *grpc.Server, port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("grpc.ListenAndServe: %w", err)
	}
	return s.Serve(lis)
}
