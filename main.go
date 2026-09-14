package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"spectra/internal/collector"
	"spectra/web"
)

// basicAuthMiddleware intercepts requests and verifies username & password
func basicAuthMiddleware(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If credentials are not configured, allow access
		if username == "" && password == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Keep health check endpoint open for Docker and Cloudflare monitoring
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Spectra Server Monitor"`)
			http.Error(w, "Unauthorized: Authentication required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	defaultPort := 5050
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			defaultPort = p
		}
	}

	defaultUser := os.Getenv("AUTH_USER")
	defaultPass := os.Getenv("AUTH_PASS")

	portFlag := flag.Int("port", defaultPort, "Port to listen on (default 5050)")
	hostFlag := flag.String("host", "0.0.0.0", "Host address to bind to (default 0.0.0.0)")
	userFlag := flag.String("user", defaultUser, "Username for HTTP Basic Auth (optional)")
	passFlag := flag.String("pass", defaultPass, "Password for HTTP Basic Auth (optional)")
	flag.Parse()

	activeUser := *userFlag
	activePass := *passFlag

	addr := fmt.Sprintf("%s:%d", *hostFlag, *portFlag)

	// Test listening synchronously to catch port conflict immediately
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("==================================================")
		fmt.Printf(" [X] ไม่สามารถเปิดใช้งานพอร์ต %d ได้: %v\n", *portFlag, err)
		fmt.Println("==================================================")
		fmt.Println(" สาเหตุที่เป็นไปได้:")
		fmt.Printf(" 1. มี Spectra หรือโปรแกรมอื่นกำลังใช้งานพอร์ต %d อยู่\n", *portFlag)
		fmt.Println(" 2. สามารถระบุพอร์ตอื่นได้ เช่น: .\\spectra.exe -port 5051")
		fmt.Println("--------------------------------------------------")
		fmt.Print(" กดปุ่ม Enter เพื่อปิดหน้าต่างนี้...")
		_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
		os.Exit(1)
	}

	// Initialize hardware collector
	col := collector.NewCollector()

	mux := http.NewServeMux()

	// Static Assets (Embedded in binary)
	fileServer := http.FileServer(web.GetFileSystem())
	mux.Handle("/", fileServer)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Real-time API Endpoint
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		// Enforce no-cache for Cloudflare proxy & CDNs
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		snap, err := col.Collect()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(snap); err != nil {
			log.Printf("Error encoding stats JSON: %v", err)
		}
	})

	// Wrap root handler with Authentication middleware
	handler := basicAuthMiddleware(activeUser, activePass, mux)

	srv := &http.Server{
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Server startup banner
	fmt.Println("==================================================")
	fmt.Printf("   Spectra — Minimal Server Monitor (Port %d)\n", *portFlag)
	fmt.Println("==================================================")
	fmt.Printf(" [✓] สถานะการทำงาน:      กำลังทำงาน (Running)\n")
	fmt.Printf(" [✓] Local URL:          http://localhost:%d\n", *portFlag)
	fmt.Printf(" [✓] Network URL:        http://%s\n", addr)
	if activeUser != "" && activePass != "" {
		fmt.Printf(" [✓] ระบบความปลอดภัย:    เปิดรหัสผ่าน Basic Auth (User: %s)\n", activeUser)
	} else {
		fmt.Println(" [!] ระบบความปลอดภัย:    สาธารณะ (ไม่มีรหัสผ่าน)")
	}
	fmt.Println(" [✓] Cloudflare Ready:   Header no-cache พร้อมใช้งาน")
	fmt.Println("--------------------------------------------------")
	fmt.Printf(" (*) เปิดหน้าเว็บได้ที่: http://localhost:%d\n", *portFlag)
	fmt.Println(" (*) อย่าเพิ่งปิดหน้าต่างนี้ เพื่อให้แดชบอร์ดทำงานต่อเนื่อง")
	fmt.Println(" (*) กด Ctrl + C ในหน้าต่างนี้เพื่อหยุดการทำงาน")
	fmt.Println("==================================================")

	// Graceful shutdown listener
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Printf("\n[X] เกิดข้อผิดพลาดขณะรันเซิร์ฟเวอร์: %v\n", err)
			fmt.Print("กดปุ่ม Enter เพื่อปิดหน้าต่าง...")
			_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
			os.Exit(1)
		}
	}()

	<-stopChan
	fmt.Println("\n\nกำลังหยุดการทำงานของ Spectra อย่างปลอดภัย...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	fmt.Println("Spectra หยุดทำงานเรียบร้อยแล้ว")
	fmt.Print("กดปุ่ม Enter เพื่อปิดหน้าต่างนี้...")
	_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
}
