# Spectra — Minimalist Server Status Monitor

Spectra เป็นเว็บแดชบอร์ดตรวจสอบสถานะเซิร์ฟเวอร์แบบเรียลไทม์ที่เขียนด้วยภาษา Go ออกแบบให้กินทรัพยากรต่ำมาก (Ultra-lightweight) เหมาะสำหรับรันใน Docker และนำไปต่อเข้ากับ Cloudflare พร้อมโดเมนของคุณ

หน้าเว็บถูกออกแบบสไตล์ Minimalist สีขาวสะอาดตา (`#FFFFFF`) เน้นข้อมูลที่เป็นประโยชน์จริง พร้อมระบบความปลอดภัยหน้าล็อกอิน (Sign In Page) และระบบปรับความถี่ Time Tick (อัตราการอัปเดตข้อมูล) ได้ตามต้องการ

---

## จุดเด่น (Features)

- **เบาและกินทรัพยากรเครื่องน้อยมาก**: Single Static Binary ขนาด ~10MB ฝังไฟล์หน้าเว็บทั้งหมดไว้ในตัว ไม่เปลือง RAM และ CPU
- **ระบบความปลอดภัยหน้าล็อกอิน (Dedicated Login Page)**: กำหนด `AUTH_USER` และ `AUTH_PASS` เพื่อล็อคแดชบอร์ด มีหน้าล็อกอินสวยงามพร้อมลูกเล่น Shake Animation, Inline Validation และปุ่ม Sign Out
- **พอร์ต 5050**: รันบนพอร์ต 5050 เป็นค่าเริ่มต้น (สามารถเปลี่ยนผ่าน `PORT=xxxx` หรือ Flag `-port xxxx`)
- **Minimalist White Web UI**: โทนสีขาวเรียบหรู ดูสบายตา อ่านค่าง่าย คมชัด สไตล์ modern enterprise
- **ปรับ Refresh Time Tick ได้ทันที**: เลือกอัตราการรีเฟรชได้ตั้งแต่ `1s`, `2s`, `5s`, `10s` หรือ `Pause` (หยุดชั่วคราว) พร้อมระบบจำค่าผ่าน `localStorage` ไม่รีเซ็ตเมื่อรีเฟรชหน้าเว็บ
- **ข้อมูลระบบครบถ้วน**:
  - **CPU**: เปอร์เซ็นต์การใช้งานรวม, จำนวน Core (Physical / Logical), ความเร็ว Clock (GHz), ค่า Load Average (1/5/15), กราฟ Sparkline ย้อนหลัง และแถบดูสถานะแยกราย Core
  - **Memory (RAM & Swap)**: ปริมาณการใช้งานจริง, พื้นที่ว่าง, แคช, เปอร์เซ็นต์ พร้อมกราฟ Sparkline
  - **Storage**: รายการไดรฟ์/พาร์ติชันทั้งหมด พร้อมจุด Mount, ขนาดที่ใช้ และความจุรวม
  - **Network I/O**: อัตราความเร็วดาวน์โหลด (Rx) และอัปโหลด (Tx) แบบเรียลไทม์ (KB/s, MB/s) ปริมาณเน็ตสะสม และจำนวน Packet
  - **System Overview**: Hostname, OS / Distro, Kernel, Architecture, Uptime, Boot Time, จำนวน Process ที่กำลังทำงาน
- **รองรับ Cloudflare เต็มรูปแบบ**: มี Header `Cache-Control: no-cache, no-store` ป้องกันการแคชสถานะเก่า และมี Endpoint `/health` สำหรับตรวจสอบสถานะ Tunnel

---

## โครงสร้างโปรเจ็กต์ (Project Structure)

```
Spectra/
├── cmd/
│   └── spectra/
│       └── main.go              # Entry point สำรอง
├── internal/
│   ├── auth/
│   │   └── auth.go              # ระบบตรวจสอบสิทธิ์และ Session Cookie (HMAC-SHA256)
│   └── collector/
│       ├── collector.go         # ระบบดึงข้อมูล Hardware/OS ด้วย gopsutil
│       └── types.go             # โครงสร้าง JSON ของข้อมูลสถิติ
├── web/
│   ├── embed.go                 # รวมไฟล์ static เข้ากับ Go binary ด้วย embed.FS
│   └── static/
│       ├── app.js               # Logic ฝั่ง Client, การคำนวณกราฟ และ Time Tick
│       ├── index.html           # โครงสร้างหน้าเว็บแดชบอร์ด Minimalist
│       ├── login.html           # หน้าล็อกอิน Sign In สไตล์ Minimalist
│       └── style.css            # ธีมสีขาวสะอาดตา (Minimal White Design)
├── Dockerfile                   # Multi-stage Docker build ขนาดเล็ก ~15MB
├── docker-compose.yml           # ตั้งค่ารันคอนเทนเนอร์พร้อม Host Mounts และ Auth
├── main.go                      # Entry point หลัก (รองรับคำสั่ง go run .)
├── run.bat                      # สคริปต์รันบน Windows แบบไม่ต้องกังวลหน้าต่างปิด
├── go.mod
├── go.sum
└── README.md
```

---

## วิธีการใช้งาน (Getting Started)

### 1. รันโดยตรงบนเครื่อง (Local Run)

```bash
# รันผ่าน Go ทันที
go run .

# หรือกำหนด Username และ Password สำหรับเข้าสู่ระบบ
go run . -user admin -pass mysecurepassword

# หรือรันไฟล์ Binary ที่คอมไพล์แล้ว
./spectra.exe
```

จากนั้นเปิดเบราว์เซอร์ไปที่: **`http://localhost:5050`**

---

### 2. รันด้วย Docker & Docker Compose (พร้อมตั้งค่า Username & Password)

เมื่อนำไปรันบนเซิร์ฟเวอร์ด้วย Docker **จำเป็นต้องกำหนด `AUTH_USER` และ `AUTH_PASS` ในไฟล์ `docker-compose.yml` เพื่อป้องกันความปลอดภัย**:

#### ขั้นตอนการตั้งค่า:

1. เปิดไฟล์ `docker-compose.yml` แล้วระบุ Username และ Password ที่ต้องการ:
   ```yaml
   services:
     spectra:
       build:
         context: .
         dockerfile: Dockerfile
       image: spectra:latest
       container_name: spectra
       restart: unless-stopped
       ports:
         - "5050:5050"
       environment:
         - PORT=5050
         # ตั้งค่า Username และ Password สำหรับเข้าสู่ระบบที่นี่:
         - AUTH_USER=admin
         - AUTH_PASS=your-secure-password
         
         # ค่าสำหรับดึงสถิติ Host จริงของ Linux
         - HOST_PROC=/host/proc
         - HOST_SYS=/host/sys
         - HOST_ETC=/host/etc
       volumes:
         - /proc:/host/proc:ro
         - /sys:/host/sys:ro
         - /etc:/host/etc:ro
         - /:/host/rootfs:ro
   ```

2. สั่งเริ่มคอนเทนเนอร์:
   ```bash
   docker compose up -d --build
   ```

3. ตรวจสอบการทำงาน:
   ```bash
   docker ps
   docker logs spectra
   ```

> **ข้อสังเกต**: 
> - เมื่อเข้าใช้งาน ระบบจะเปิดหน้า **Sign In** ให้กรอก Username และ Password ที่กำหนดไว้
> - ระบบใช้ Session Cookie ที่ปลอดภัย อยู่ได้นาน 30 วัน และมีปุ่ม **Sign Out** บนแถบ Header สำหรับออกจากระบบ
> - Endpoint `/health` จะเปิดไว้เสมอเพื่อให้ Docker Healthcheck ทำงานได้ต่อเนื่องโดยไม่ต้องติดสิทธิ์

---

### 3. การเชื่อมต่อกับ Cloudflare และโดเมนของคุณ

มี 2 วิธีหลักในการนำ Spectra ไปใช้งานผ่าน Cloudflare:

#### วิธีที่ 1: ใช้ Cloudflare Tunnel (`cloudflared`) — **แนะนำที่สุด (ปลอดภัยและไม่ต้อง Forward Port)**
1. ติดตั้ง `cloudflared` บนเซิร์ฟเวอร์ หรือสร้าง Tunnel ผ่านหน้าเว็บ Cloudflare Zero Trust Dashboard
2. ในส่วน **Public Hostname**:
   - **Service Type**: เลือกเป็น **`HTTP`** (ห้ามเลือก HTTPS เนื่องจากตัว Spectra ให้บริการเป็น HTTP แล้ว Cloudflare จะเป็นตัวครอบ SSL ให้เอง)
   - **URL**: ระบุเป็น **`127.0.0.1:5050`**
3. บันทึกและเปิดเข้าผ่านโดเมนของคุณได้ทันที เช่น `https://status.yourdomain.com`

#### วิธีที่ 2: Reverse Proxy (Nginx / Caddy)
หากมี Public IP และต้องการรับผ่าน Nginx:
```nginx
server {
    listen 80;
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

## Web & API Endpoints

- `GET /` — หน้าเว็บแดชบอร์ดหลัก (ต้องล็อกอินหากเปิดใช้งาน Auth)
- `GET /login` — หน้าล็อกอิน Sign In สไตล์ Minimalist
- `POST /api/login` — Endpoint สำหรับตรวจสอบ Username/Password และสร้าง Session Cookie
- `POST /api/logout` — ออกจากระบบ ทำลาย Session Cookie
- `GET /api/stats` — ส่งข้อมูลสถิติของเซิร์ฟเวอร์แบบ JSON Snapshot แบบเรียลไทม์
- `GET /health` — Health check endpoint สำหรับ Docker หรือ Cloudflare Probe (เปิดไว้เสมอโดยไม่ต้องยืนยันตัวตน)
