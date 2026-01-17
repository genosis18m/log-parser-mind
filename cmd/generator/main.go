// Package main provides a sample log data generator for testing.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"
)

var logTemplates = []string{
	"ERROR: Connection refused to database at %s:%d",
	"WARN: High memory usage detected: %d%% on server %s",
	"INFO: Request processed in %dms for user %s",
	"ERROR: Failed to authenticate user %s from IP %s",
	"INFO: Service started on port %d",
	"WARN: Disk usage at %d%% on volume %s",
	"ERROR: Timeout after %ds waiting for response from %s",
	"INFO: Successfully deployed version %s to %s",
	"ERROR: Out of memory error on pod %s",
	"WARN: SSL certificate expires in %d days for %s",
	"INFO: Backup completed: %d files, %dMB total",
	"ERROR: Database query failed: %s",
	"INFO: Cache hit rate: %d%% for service %s",
	"WARN: Rate limit exceeded for API key %s",
	"ERROR: Connection pool exhausted: %d/%d connections in use",
}

var ips = []string{"192.168.1.1", "10.0.0.5", "172.16.0.10", "10.0.1.15", "192.168.2.20"}
var servers = []string{"server-01", "server-02", "web-prod-1", "api-prod-2", "db-master"}
var users = []string{"john", "jane", "admin", "service-account", "bot-user"}
var services = []string{"auth-service", "payment-api", "user-service", "order-service", "notification"}
var versions = []string{"v1.2.3", "v1.2.4", "v2.0.0", "v2.0.1-beta", "v1.9.9"}

func generateLog() string {
	template := logTemplates[rand.Intn(len(logTemplates))]
	
	switch {
	case containsPlaceholder(template, "%s:%d"):
		return fmt.Sprintf(template, ips[rand.Intn(len(ips))], 5432+rand.Intn(100))
	case containsPlaceholder(template, "%d%% on server"):
		return fmt.Sprintf(template, 50+rand.Intn(50), servers[rand.Intn(len(servers))])
	case containsPlaceholder(template, "for user %s"):
		return fmt.Sprintf(template, 10+rand.Intn(500), users[rand.Intn(len(users))])
	case containsPlaceholder(template, "from IP"):
		return fmt.Sprintf(template, users[rand.Intn(len(users))], ips[rand.Intn(len(ips))])
	case containsPlaceholder(template, "port %d"):
		return fmt.Sprintf(template, 8080+rand.Intn(20))
	case containsPlaceholder(template, "volume"):
		return fmt.Sprintf(template, 70+rand.Intn(30), "/dev/sda"+fmt.Sprint(rand.Intn(5)))
	case containsPlaceholder(template, "waiting for"):
		return fmt.Sprintf(template, 5+rand.Intn(30), services[rand.Intn(len(services))])
	case containsPlaceholder(template, "version"):
		return fmt.Sprintf(template, versions[rand.Intn(len(versions))], servers[rand.Intn(len(servers))])
	case containsPlaceholder(template, "pod"):
		return fmt.Sprintf(template, services[rand.Intn(len(services))]+"-"+fmt.Sprint(rand.Intn(10)))
	case containsPlaceholder(template, "expires"):
		return fmt.Sprintf(template, rand.Intn(30), services[rand.Intn(len(services))]+".example.com")
	case containsPlaceholder(template, "files"):
		return fmt.Sprintf(template, 100+rand.Intn(1000), 50+rand.Intn(500))
	case containsPlaceholder(template, "query failed"):
		return fmt.Sprintf(template, "syntax error near 'SELECT'")
	case containsPlaceholder(template, "hit rate"):
		return fmt.Sprintf(template, 80+rand.Intn(20), services[rand.Intn(len(services))])
	case containsPlaceholder(template, "API key"):
		return fmt.Sprintf(template, "ak_"+randomString(8))
	case containsPlaceholder(template, "connections"):
		max := 50 + rand.Intn(50)
		return fmt.Sprintf(template, max-rand.Intn(10), max)
	default:
		return template
	}
}

func containsPlaceholder(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func sendLog(endpoint, log, source string) error {
	payload := map[string]string{
		"log":    log,
		"source": source,
	}
	
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	return nil
}

func main() {
	endpoint := flag.String("endpoint", "http://localhost:8091/ingest", "Ingestion endpoint")
	count := flag.Int("count", 100, "Number of logs to generate")
	rate := flag.Int("rate", 10, "Logs per second")
	source := flag.String("source", "sample-generator", "Log source name")
	dryRun := flag.Bool("dry-run", false, "Print logs instead of sending")
	flag.Parse()
	
	rand.Seed(time.Now().UnixNano())
	
	fmt.Printf("Generating %d logs at %d/sec to %s\n", *count, *rate, *endpoint)
	
	interval := time.Second / time.Duration(*rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	sent := 0
	errors := 0
	
	for i := 0; i < *count; i++ {
		<-ticker.C
		
		log := generateLog()
		
		if *dryRun {
			fmt.Printf("[%s] %s\n", *source, log)
		} else {
			if err := sendLog(*endpoint, log, *source); err != nil {
				errors++
				fmt.Fprintf(os.Stderr, "Error sending log: %v\n", err)
			} else {
				sent++
			}
		}
		
		if (i+1) % 10 == 0 {
			fmt.Printf("Progress: %d/%d (errors: %d)\n", i+1, *count, errors)
		}
	}
	
	fmt.Printf("\nComplete! Sent: %d, Errors: %d\n", sent, errors)
}
