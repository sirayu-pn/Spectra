# Spectra — Minimalist Server Status Monitor

Spectra เป็นเว็บแดชบอร์ดตรวจสอบสถานะเซิร์ฟเวอร์แบบเรียลไทม์ที่เขียนด้วยภาษา Go ออกแบบให้กินทรัพยากรต่ำมาก (Ultra-lightweight) เหมาะสำหรับรันใน Docker และนำไปต่อเข้ากับ Cloudflare พร้อมโดเมนของคุณ

หน้าเว็บถูกออกแบบสไตล์ Minimalist สีขาวสะอาดตา (`#FFFFFF`) เน้นข้อมูลที่เป็นประโยชน์จริง ไม่มีกราฟฟิกหรือสคริปต์ที่ไม่จำเป็น และสามารถปรับความถี่ Time Tick (อัตราการอัปเดตข้อมูล) ได้ตามต้องการ

---

## จุดเด่น (Features)

- **เบาและกินทรัพยากรเครื่องน้อยมาก**: Single Static Binary ขนาด ~10MB ฝังไฟล์หน้าเว็บทั้งหมดไว้ในตัว ไม่เปลือง RAM และ CPU
- **พอร์ต 5050**: รันบนพอร์ต 5050 เป็นค่าเริ่มต้น (สามารถเปลี่ยนผ่าน `PORT=xxxx` หรือ `-port xxxx`)
- **Minimalist White Web UI**: โทนสีขาวเรียบหรู ดูสบายตา อ่านค่าง่าย คมชัด สไตล์ modern enterprise
- **ปรับ Refresh Time Tick ได้ทันที**: เลือกอัตราการรีเฟรชได้ตั้งแต่ `1s`, `2s`, `5s`, `10s` หรือ `Pause` (หยุดชั่วคราว) พร้อมปุ่มรีเฟรชมือ
- **ข้อมูลระบบครบถ้วน**:
  - **CPU**: เปอร์เซ็นต์การใช้งานรวม, จำนวน Core (Physical / Logical), ความเร็ว Clock (GHz), ค่า Load Average (1/5/15), กราฟ Sparkline ย้อนหลัง และแถบดูสถานะแยกราย Core
  - **Memory (RAM & Swap)**: ปริมาณการใช้งานจริง, พื้นที่ว่าง, แคช, เปอร์เซ็นต์ พร้อมกราฟ Sparkline
  - **Storage**: รายการไดรฟ์/พาร์ติชันทั้งหมด พร้อมจุด Mount, ขนาดที่ใช้ และความจุรวม
  - **Network I/O**: อัตราความเร็วดาวน์โหลด (Rx) และอัปโหลด (Tx) แบบเรียลไทม์ (KB/s, MB/s) ปริมาณเน็ตสะสม และจำนวน Packet
  - **System Overview**: Hostname, OS / Distro, Kernel, Architecture, Uptime, จำนวน Process ที่กำลังทำงาน
- **รองรับ Cloudflare เต็มรูปแบบ**: มี Header `Cache-Control: no-cache, no-store` ป้องกันการแคชสถานะเก่า และมี Endpoint `/health` สำหรับตรวจสอบสถานะ Tunnel

---

## โครงสร้างโปรเจ็กต์ (Project Structure)

```
Spectra/
├── cmd/
│   └── spectra/
│       └── main.go              # Entry point ของโปรแกรม และ Web Server
├── internal/
│   └── collector/
│       ├── collector.go         # ระบบดึงข้อมูล Hardware/OS ด้วย gopsutil
│       └── types.go             # โครงสร้าง JSON ของข้อมูลสถิติ
├── web/
│   ├── embed.go                 # รวมไฟล์ static เข้ากับ Go binary ด้วย embed.FS
│   └── static/
│       ├── app.js               # Logic ฝั่ง Client, การคำนวณกราฟ และ Time Tick
│       ├── index.html           # โครงสร้างหน้าเว็บ Minimalist
│       └── style.css            # ธีมสีขาวสะอาดตา (Minimal White Design)
├── Dockerfile                   # Multi-stage Docker build ขนาดเล็ก ~15MB
├── docker-compose.yml           # ตั้งค่ารันคอนเทนเนอร์พร้อม Host Mounts
├── go.mod
├── go.sum
└── README.md
```

---

## วิธีการใช้งาน (Getting Started)

### 1. รันโดยตรงบนเครื่อง (Local Run)

```bash
# รันผ่าน Go
go run ./cmd/spectra

# หรือรันไฟล์ Binary ที่คอมไพล์แล้ว
./spectra.exe
```

จากนั้นเปิดเบราว์เซอร์ไปที่: **`http://localhost:5050`**

หากต้องการเปลี่ยนพอร์ต:
```bash
go run ./cmd/spectra -port 8080
# หรือกำหนด Environment
PORT=8080 ./spectra
```

---

### 2. รันด้วย Docker & Docker Compose

#### สร้างและเริ่มคอนเทนเนอร์:
```bash
docker compose up -d --build
```

ตรวจสอบการทำงาน:
```bash
docker ps
docker logs spectra
```

> **คำแนะนำสำหรับ Linux Host**: `docker-compose.yml` ได้ตั้งค่าเมานต์ `/proc` และ `/sys` จาก Host เอาไว้แล้ว ทำให้เมื่อรันใน Docker จะสามารถอ่านข้อมูล CPU, RAM และเน็ตเวิร์กของเครื่อง Host จริงได้ถูกต้องสมบูรณ์

---

### 3. การเชื่อมต่อกับ Cloudflare และโดเมนของคุณ

มี 2 วิธีหลักในการนำ Spectra ไปใช้งานผ่าน Cloudflare:

#### วิธีที่ 1: ใช้ Cloudflare Tunnel (`cloudflared`) — **แนะนำที่สุด (ปลอดภัยและไม่ต้อง Forward Port)**
1. ติดตั้ง `cloudflared` บนเซิร์ฟเวอร์ของคุณ หรือใช้ Cloudflare Zero Trust Dashboard
2. สร้าง Tunnel ชี้มาที่พอร์ต `5050` ของเครื่อง:
   ```bash
   cloudflared tunnel route dns <tunnel-name> status.yourdomain.com
   ```
3. ในไฟล์คอนฟิก `config.yml` ของ Cloudflare Tunnel:
   ```yaml
   ingress:
     - hostname: status.yourdomain.com
       service: http://localhost:5050
     - service: http_status:404
   ```
4. เริ่มรัน Tunnel:
   ```bash
   cloudflared tunnel run <tunnel-name>
   ```
   คุณจะสามารถเข้าผ่าน `https://status.yourdomain.com` ได้ทันที โดย Cloudflare จะจัดการ SSL Certificate ให้อัตโนมัติ

#### วิธีที่ 2: Forward Port หรือ Reverse Proxy (Nginx / Caddy)
หากเซิร์ฟเวอร์มี Public IP และต้องการเปิดพอร์ต 5050 หรือส่งผ่าน Reverse Proxy:
1. ตั้งค่า DNS บน Cloudflare ชี้ A Record มายัง IP เซิร์ฟเวอร์ของคุณ (เปิด Proxy คลาวด์สีส้ม)
2. สังเกตว่า Cloudflare รองรับพอร์ตมาตรฐาน HTTP (80, 8080, 8880, 2052, 2082, 2086, 2095) และ HTTPS (443, 2053, 2083, 2087, 2096, 8443)
3. **หากต้องการใช้พอร์ต 5050 เข้าผ่านโดเมน**: แนะนำให้ใช้ **Nginx / Caddy** หรือ **Cloudflare Tunnel (วิธีที่ 1)** ทำ Reverse Proxy จากพอร์ต 443 ภายในโดเมนส่งต่อไปยัง `http://127.0.0.1:5050`
   
   ตัวอย่าง Nginx:
   ```nginx
   server {
       server_name status.yourdomain.com;

       location / {
           proxy_pass http://127.0.0.1:5050;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $scheme;
       }
   }
   ```

---

## API Endpoints

- `GET /` — หน้าเว็บแดชบอร์ดหลัก
- `GET /api/stats` — ส่งข้อมูลสถิติของเซิร์ฟเวอร์แบบ JSON Snapshot แบบเรียลไทม์
- `GET /health` — Health check endpoint สำหรับ Docker หรือ Cloudflare Probe
