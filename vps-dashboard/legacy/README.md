# Legacy code

Folder ini berisi implementasi lama yang sudah tidak digunakan dalam deployment baru (Docker Compose stack).
Dipertahankan untuk referensi historis saja.

## backend-node/

Implementasi awal backend dalam Node.js + Express. Sudah digantikan sepenuhnya oleh `backend-go/`.
Tidak ikut dibangun atau dijalankan oleh `docker-compose.yml`.

Endpoint yang dulu ada di sini (`/docker`, `/system`, `/generator`) sekarang dilayani oleh `backend-go/`
dengan auth JWT, audit log, dan validasi command yang lebih ketat.

JANGAN deploy ulang folder ini. Backend ini menerima command tanpa auth dan rentan command injection.
