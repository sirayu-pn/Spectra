package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"spectra/internal/auth"
	"spectra/internal/collector"
	"spectra/web"
)

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
	userFlag := flag.String("user", defaultUser, "Username for authentication (optional)")
	passFlag := flag.String("pass", defaultPass, "Password for authentication (optional)")
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

	// Initialize hardware collector & auth manager
	col := collector.NewCollector()
	authMgr := auth.NewAuthManager(activeUser, activePass)

	// Start centralized background collector ticker (1-second tick)
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	col.Start(bgCtx, 1*time.Second)

	mux := http.NewServeMux()

	// 1. Health check endpoint (always unauthenticated for Docker and Cloudflare)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// 2. Login page
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if !authMgr.IsEnabled() {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if cookie, err := r.Cookie(auth.CookieName); err == nil {
			if authMgr.VerifySessionToken(cookie.Value) {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}

		loginFile, err := web.GetFileSystem().Open("login.html")
		if err != nil {
			http.Error(w, "Login page unavailable", http.StatusInternalServerError)
			return
		}
		defer loginFile.Close()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = io.Copy(w, loginFile)
	})

	// 3. Login API (Handles authentication form submit)
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error":"Method not allowed"}`))
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Invalid request format"}`))
			return
		}

		if !authMgr.ValidateCredentials(req.Username, req.Password) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid username or password"}`))
			return
		}

		token := authMgr.GenerateSessionToken()
		http.SetCookie(w, &http.Cookie{
			Name:     auth.CookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(auth.SessionMaxAge.Seconds()),
		})

		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// 4. Logout Handler
	logoutHandler := func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     auth.CookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
		})
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	}
	mux.HandleFunc("/api/logout", logoutHandler)
	mux.HandleFunc("/logout", logoutHandler)

	// 5. Server-Sent Events (SSE) Stream Endpoint
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Disable response write deadline for long-lived SSE streaming
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Time{})

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Push initial state immediately so dashboard renders without waiting for the next tick
		if initialJSON := col.GetLatestJSON(); len(initialJSON) > 0 {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", initialJSON)
			flusher.Flush()
		}

		subCh, unsubscribe := col.Broker().Subscribe()
		defer unsubscribe()

		clientDone := r.Context().Done()
		for {
			select {
			case <-clientDone:
				return
			case data, ok := <-subCh:
				if !ok {
					return
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	})

	// 6. Real-time API Endpoint (Cached for zero-overhead polling)
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if cachedJSON := col.GetLatestJSON(); len(cachedJSON) > 0 {
			_, _ = w.Write(cachedJSON)
			return
		}

		// Fallback if cache not ready yet
		snap, err := col.Collect()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(snap); err != nil {
			log.Printf("Error encoding stats JSON: %v", err)
		}
	})

	// 7. Static Assets (with no-cache headers to prevent CSS/JS stale cache)
	fileServer := http.FileServer(web.GetFileSystem())
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".css") || strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		fileServer.ServeHTTP(w, r)
	}))

	// Wrap mux with session authentication middleware
	handler := authMgr.Middleware(mux)

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
	if authMgr.IsEnabled() {
		fmt.Printf(" [✓] ระบบความปลอดภัย:    เปิดใช้งานหน้า Login (User: %s)\n", activeUser)
	} else {
		fmt.Println(" [!] ระบบความปลอดภัย:    สาธารณะ (ไม่มีรหัสผ่าน)")
	}
	fmt.Println(" [✓] Real-time Stream:   SSE (/api/stream) เปิดใช้งาน")
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

	bgCancel() // stop background collector ticker

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	fmt.Println("Spectra หยุดทำงานเรียบร้อยแล้ว")
	fmt.Print("กดปุ่ม Enter เพื่อปิดหน้าต่างนี้...")
	_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
}
