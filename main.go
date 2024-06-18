package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "yadro-test-server/proto"
)

type server struct {
	pb.UnimplementedYadroServiceServer
}

func (s *server) ChangeHostName(ctx context.Context, req *pb.ChangeHostNameRequest) (*pb.ChangeHostNameResponse, error) {
	newHostname := req.GetHostname()
	cmd := exec.Command("hostnamectl", "set-hostname", newHostname)
	err := cmd.Run()
	if err != nil {
		log.Printf("Failed to change hostname: %v", err)
		return nil, err
	}
	log.Printf("Changing host name to: %s", req.GetHostname())
	return &pb.ChangeHostNameResponse{Message: "Host name changed successfully"}, nil
}

func (s *server) ModifyDNS(ctx context.Context, req *pb.ModifyDNSRequest) (*pb.ModifyDNSResponse, error) {
	action := req.GetAction()
	ip := req.GetIp()
	log.Print(req)
	log.Printf("%s, %s", action, ip)
	resolvConf := "/etc/resolv.conf"
	content, err := os.ReadFile(resolvConf)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", resolvConf, err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	modified := false

	for _, line := range lines {
		if strings.HasPrefix(line, "nameserver ") {
			if action == "delete" && strings.TrimPrefix(line, "nameserver ") == ip {
				modified = true
				continue
			}
		}
		newLines = append(newLines, line)
	}

	if action == "add" {
		newLines = append(newLines, "nameserver "+ip)
		modified = true
	}

	if !modified {
		return nil, fmt.Errorf("DNS server %s not found or no changes made", ip)
	}

	err = os.WriteFile(resolvConf, []byte(strings.Join(newLines, "\n")), 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write %s: %v", resolvConf, err)
	}

	return &pb.ModifyDNSResponse{Message: "DNS modified successfully"}, nil
}

func (s *server) ListDNS(ctx context.Context, req *pb.ListDNSRequest) (*pb.ListDNSResponse, error) {
	resolvConf := "/etc/resolv.conf"
	content, err := os.ReadFile(resolvConf)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	dnsServers := []string{}
	for _, line := range lines {
		if strings.HasPrefix(line, "nameserver ") {
			dnsServers = append(dnsServers, strings.TrimPrefix(line, "nameserver "))
		}
	}

	return &pb.ListDNSResponse{DnsList: dnsServers}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterYadroServiceServer(s, &server{})

	reflection.Register(s)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err = pb.RegisterYadroServiceHandlerFromEndpoint(ctx, mux, "localhost:50051", opts)
	if err != nil {
		log.Fatalf("failed to start HTTP gateway: %v", err)
	}

	go func() {
		log.Printf("gRPC server listening on %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	log.Printf("HTTP server listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("failed to serve HTTP: %v", err)
	}
}
