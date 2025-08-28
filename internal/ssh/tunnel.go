package ssh

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"fuel-monitor-api/internal/config"

	"golang.org/x/crypto/ssh"
)

func SetupTunnel(cfg *config.Config) (*ssh.Client, int, error) {
	// SSH client configuration
	sshConfig := &ssh.ClientConfig{
		User: cfg.SSH.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.SSH.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, use proper host key verification
		Timeout:         30 * time.Second,
	}

	// Connect to SSH server
	log.Printf("Connecting to SSH server: %s:22", cfg.SSH.Host)
	sshClient, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", cfg.SSH.Host), sshConfig)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to connect to SSH server: %w", err)
	}

	// Find available local port
	localPort, err := findAvailablePort()
	if err != nil {
		sshClient.Close()
		return nil, 0, fmt.Errorf("failed to find available port: %w", err)
	}

	// Start local listener
	localListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		sshClient.Close()
		return nil, 0, fmt.Errorf("failed to start local listener: %w", err)
	}

	log.Printf("SSH tunnel established: local port %d -> %s:%d",
		localPort, cfg.SSH.RemoteBindHost, cfg.SSH.RemoteBindPort)

	// Handle tunnel connections
	go func() {
		defer localListener.Close()
		for {
			localConn, err := localListener.Accept()
			if err != nil {
				log.Printf("Failed to accept local connection: %v", err)
				continue
			}

			go handleTunnelConnection(sshClient, localConn, cfg.SSH.RemoteBindHost, cfg.SSH.RemoteBindPort)
		}
	}()

	return sshClient, localPort, nil
}

func handleTunnelConnection(sshClient *ssh.Client, localConn net.Conn, remoteHost string, remotePort int) {
	defer localConn.Close()

	// Connect to remote server through SSH tunnel
	remoteAddr := fmt.Sprintf("%s:%d", remoteHost, remotePort)
	remoteConn, err := sshClient.Dial("tcp", remoteAddr)
	if err != nil {
		log.Printf("Failed to dial remote address %s: %v", remoteAddr, err)
		return
	}
	defer remoteConn.Close()

	// Bidirectional copy
	go func() {
		defer remoteConn.Close()
		defer localConn.Close()
		io.Copy(remoteConn, localConn)
	}()

	defer remoteConn.Close()
	defer localConn.Close()
	io.Copy(localConn, remoteConn)
}

func findAvailablePort() (int, error) {
	// Let the system find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}
