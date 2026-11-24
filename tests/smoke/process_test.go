//go:build smoke

package smoke

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

type process struct {
	cmd     *exec.Cmd
	log     *os.File
	done    chan struct{}
	waitErr error
	stopped bool
}

func environment(overrides map[string]string) []string {
	values := map[string]string{}
	for _, item := range os.Environ() {
		key, value, _ := strings.Cut(item, "=")
		values[key] = value
	}
	for key, value := range overrides {
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func binary(t *testing.T, service string) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "bin", service)
	if _, err := os.Stat(path); err != nil {
		t.Fatal("run make build before process smoke tests")
	}
	return path
}

func start(t *testing.T, service string, overrides map[string]string) *process {
	t.Helper()
	log, err := os.CreateTemp(t.TempDir(), "service-*.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	cmd := exec.Command(binary(t, service))
	cmd.Env, cmd.Stdout, cmd.Stderr = environment(overrides), log, log
	if err := cmd.Start(); err != nil {
		t.Fatal("could not start COT executable")
	}
	p := &process{cmd: cmd, log: log, done: make(chan struct{})}
	go func() { p.waitErr = cmd.Wait(); close(p.done) }()
	t.Cleanup(func() { p.stop(t) })
	return p
}

func (p *process) stop(t *testing.T) {
	t.Helper()
	if p.stopped {
		return
	}
	p.stopped = true
	select {
	case <-p.done:
		t.Error("service exited before shutdown was requested")
		return
	default:
	}
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Error("could not signal owned COT process")
	}
	select {
	case <-p.done:
		if p.waitErr != nil {
			body, _ := os.ReadFile(p.log.Name())
			code := "unknown"
			for _, line := range strings.Split(string(body), "\n") {
				var record map[string]any
				if json.Unmarshal([]byte(line), &record) == nil {
					if value, ok := record["code"].(string); ok {
						code = value
					}
				}
			}
			t.Errorf("graceful shutdown returned a failure (code=%s)", code)
		}
	case <-time.After(13 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Error("graceful shutdown exceeded deadline")
	}
}

func address(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	value := listener.Addr().String()
	_ = listener.Close()
	return value
}

var requestID = regexp.MustCompile("^[a-f0-9]{32}$")

func request(t *testing.T, client *http.Client, base, method, path, token string, want int) map[string]any {
	t.Helper()
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Player-ID", "spoofed-player")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal("HTTP request to owned COT process failed")
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("%s %s returned %d; want %d", method, path, response.StatusCode, want)
	}
	if !requestID.MatchString(response.Header.Get("X-Request-ID")) {
		t.Fatal("missing safe request reference")
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("missing response cache policy")
	}
	var body map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&body); err != nil {
		t.Fatal("invalid JSON response")
	}
	return body
}

func awaitLiveness(t *testing.T, client *http.Client, base string, p *process) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-p.done:
			t.Fatal("service exited during startup")
		default:
		}
		response, err := client.Get(base + "/health/live")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("service did not become live before deadline")
}

func requireLocal(t *testing.T) {
	t.Helper()
	if env := os.Getenv("COT_ENV"); env != "local" && env != "test" {
		t.Fatal("smoke tests require explicit local/test environment")
	}
	if os.Getenv("COT_DEV_AUTH_TOKEN") == "" {
		t.Fatal("smoke tests require local credentials; use make test-smoke")
	}
}

func TestServices(t *testing.T) {
	requireLocal(t)
	client := &http.Client{Timeout: 4 * time.Second}
	for _, service := range []string{"game-service", "trading-service"} {
		t.Run(service, func(t *testing.T) {
			bind := address(t)
			p := start(t, service, map[string]string{"COT_HTTP_ADDR": bind})
			base := "http://" + bind
			awaitLiveness(t, client, base, p)
			live := request(t, client, base, "GET", "/health/live", "", 200)
			if live["service"] != service {
				t.Fatal("incorrect service identity")
			}
			ready := request(t, client, base, "GET", "/health/ready", "", 200)
			if ready["ready"] != true {
				t.Fatal("migrated database is not ready")
			}
			request(t, client, base, "GET", "/v1/me", "", 401)
			request(t, client, base, "GET", "/v1/me", "incorrect-token", 401)
			me := request(t, client, base, "GET", "/v1/me", os.Getenv("COT_DEV_AUTH_TOKEN"), 200)
			principal, ok := me["principal"].(map[string]any)
			if !ok || principal["playerId"] != os.Getenv("COT_DEV_PLAYER_ID") {
				t.Fatal("identity was not derived from server configuration")
			}
			request(t, client, base, "POST", "/health/live", "", 405)
			request(t, client, base, "GET", "/missing?private=do-not-log", "", 404)
			p.stop(t)
			body, err := os.ReadFile(p.log.Name())
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{os.Getenv("COT_DEV_AUTH_TOKEN"), os.Getenv("COT_DATABASE_URL"), os.Getenv("COT_POSTGRES_PASSWORD"), "do-not-log"} {
				if secret != "" && strings.Contains(string(body), secret) {
					t.Fatal("service logs exposed private input")
				}
			}
			scanner := bufio.NewScanner(strings.NewReader(string(body)))
			lines := 0
			for scanner.Scan() {
				var record map[string]any
				if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
					t.Fatal("service log is not structured JSON")
				}
				if record["service"] != service {
					t.Fatal("log lacks service identity")
				}
				if strings.Count(scanner.Text(), "\"service\":") != 1 {
					t.Fatal("duplicate service log field")
				}
				lines++
			}
			if err := scanner.Err(); err != nil || lines == 0 {
				t.Fatal("service logs missing or unreadable")
			}
		})
	}
}

func TestDatabaseOutagePreservesLiveness(t *testing.T) {
	requireLocal(t)
	bind := address(t)
	unavailableDB := address(t)
	p := start(t, "game-service", map[string]string{
		"COT_HTTP_ADDR":    bind,
		"COT_DATABASE_URL": "postgres://cot:unused-password@" + unavailableDB + "/cot?sslmode=disable",
	})
	base := "http://" + bind
	client := &http.Client{Timeout: 4 * time.Second}
	awaitLiveness(t, client, base, p)
	ready := request(t, client, base, "GET", "/health/ready", "", 503)
	dependencies, ok := ready["dependencies"].(map[string]any)
	if !ok || dependencies["database"] != "database_unavailable" {
		t.Fatal("database failure was not classified")
	}
	request(t, client, base, "GET", "/health/live", "", 200)
}

func TestGracefulShutdownWithIncompleteHeaders(t *testing.T) {
	requireLocal(t)
	bind := address(t)
	p := start(t, "game-service", map[string]string{"COT_HTTP_ADDR": bind})
	client := &http.Client{Timeout: 4 * time.Second}
	awaitLiveness(t, client, "http://"+bind, p)
	connection, err := net.DialTimeout("tcp", bind, time.Second)
	if err != nil {
		t.Fatal("could not open owned test connection")
	}
	defer connection.Close()
	// Keep a request incomplete. The server must let its header deadline expire
	// and finish shutdown within the larger process shutdown allowance.
	if _, err := io.WriteString(connection, "GET /health/live HTTP/1.1\r\n"); err != nil {
		t.Fatal("could not send incomplete request")
	}
	p.stop(t)
}

func TestUnsafeConfigurationFailsWithoutSecrets(t *testing.T) {
	requireLocal(t)
	for _, overrides := range []map[string]string{
		{"COT_ENV": "production"},
		{"COT_DATABASE_URL": "postgres://cot:smoke-private-password@%bad/cot"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd := exec.CommandContext(ctx, binary(t, "game-service"))
		cmd.Env = environment(overrides)
		output, err := cmd.CombinedOutput()
		timedOut := ctx.Err() != nil
		cancel()
		if timedOut {
			t.Fatal("unsafe configuration did not fail promptly")
		}
		if err == nil {
			t.Fatal("unsafe configuration started successfully")
		}
		if !strings.Contains(string(output), "configuration error:") {
			t.Fatal("missing useful configuration failure")
		}
		if strings.Contains(string(output), "smoke-private-password") {
			t.Fatal("configuration failure exposed credentials")
		}
	}
}
