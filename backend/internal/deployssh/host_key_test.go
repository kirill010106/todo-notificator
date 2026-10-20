package deployssh_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func testSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	return signer
}

func TestDeploymentRejectsSubstitutedHostKey(t *testing.T) {
	python := "python3"
	if runtime.GOOS == "windows" {
		python = "python"
	}
	directory := t.TempDir()
	pinFile := filepath.Join(directory, "known_hosts")
	emptyConfig := filepath.Join(directory, "ssh_config")
	require.NoError(t, os.WriteFile(emptyConfig, nil, 0o600))
	trusted, substituted := testSigner(t), testSigner(t)
	helper, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", "configure_deploy_ssh.py"))
	require.NoError(t, err)
	configure := exec.Command(python, helper, "--known-hosts", pinFile)
	configure.Env = append(os.Environ(), "SERVER_IP=deploy.example.test",
		"SSH_HOST_PUBLIC_KEY="+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(trusted.PublicKey()))))
	output, err := configure.CombinedOutput()
	require.NoError(t, err, "%s", output)

	for _, test := range []struct {
		name   string
		signer ssh.Signer
		allow  bool
	}{{"verified server", trusted, true}, {"substituted server", substituted, false}} {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			authenticated := make(chan bool, 1)
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					authenticated <- false
					return
				}
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
				server := &ssh.ServerConfig{NoClientAuth: true}
				server.AddHostKey(test.signer)
				sshConnection, channels, requests, err := ssh.NewServerConn(connection, server)
				authenticated <- err == nil
				if err != nil {
					return
				}
				defer sshConnection.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					if incoming.ChannelType() != "session" {
						_ = incoming.Reject(ssh.UnknownChannelType, "session required")
						continue
					}
					channel, requests, err := incoming.Accept()
					if err != nil {
						return
					}
					for request := range requests {
						if request.Type == "exec" {
							_ = request.Reply(true, nil)
							_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							_ = channel.Close()
							return
						}
						_ = request.Reply(false, nil)
					}
				}
			}()
			_, port, err := net.SplitHostPort(listener.Addr().String())
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			client := exec.CommandContext(ctx, "ssh", "-F", emptyConfig, "-p", port,
				"-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile="+pinFile,
				"-o", "GlobalKnownHostsFile="+os.DevNull, "-o", "UpdateHostKeys=no",
				"-o", "VerifyHostKeyDNS=no", "-o", "HostKeyAlias=deploy.example.test",
				"-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none",
				"-o", "IdentityFile=none", "-o", "User=codex-test",
				"-o", "ConnectTimeout=5", "127.0.0.1", "true")
			output, err := client.CombinedOutput()
			if test.allow {
				require.NoError(t, err, "%s", output)
			} else {
				require.Error(t, err)
				require.Contains(t, strings.ToLower(string(output)), "host key verification failed")
			}
			select {
			case accepted := <-authenticated:
				require.Equal(t, test.allow, accepted, "a substituted server must be rejected before authentication")
			case <-ctx.Done():
				t.Fatal("SSH handshake did not complete")
			}
		})
	}
}
