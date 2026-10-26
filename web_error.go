package main

import (
	"context"
	"errors"
	"net"
	"strings"
)

// describeWebJobError returns bounded, safe diagnostic information to the
// authenticated Web UI. Never send a raw error: it may contain local paths,
// source URLs, transport details or other operational information.
func describeWebJobError(err error) (code, message string) {
	switch {
	case errors.Is(err, context.Canceled):
		return "job_cancelled", "งานถูกยกเลิกแล้ว"
	case errors.Is(err, context.DeadlineExceeded):
		return "job_timeout", "งานใช้เวลาเกินกำหนด โปรดลองใหม่หรือลดจำนวนงานพร้อมกัน"
	}

	reason := strings.ToLower(err.Error())
	switch {
	case strings.Contains(reason, "403 forbidden"), strings.Contains(reason, "401 unauthorized"):
		return "source_access_denied", "เว็บไซต์ต้นทางปฏิเสธการเข้าถึงข้อมูลหนังสือหรือภาพ (HTTP 401/403) กรุณาตรวจสอบสิทธิ์การเข้าถึง"
	case strings.Contains(reason, "404 not found"):
		return "source_not_found", "ไม่พบข้อมูลหนังสือหรือไฟล์ภาพที่ร้องขอ (HTTP 404)"
	case strings.Contains(reason, "429 too many requests"):
		return "source_rate_limited", "เว็บไซต์ต้นทางจำกัดจำนวนคำขอ (HTTP 429) กรุณาลองใหม่ภายหลัง"
	case strings.Contains(reason, "unsupported image extension"),
		strings.Contains(reason, "invalid image data"),
		strings.Contains(reason, "missing downloaded image"):
		return "image_invalid", "ข้อมูลภาพจากหนังสือไม่สมบูรณ์หรือเป็นรูปแบบที่ระบบยังไม่รองรับ"
	case strings.Contains(reason, "invalid anyflip book configuration"),
		strings.Contains(reason, "book metadata"),
		strings.Contains(reason, "could not find page count"):
		return "metadata_invalid", "ไม่สามารถอ่านจำนวนหน้าหรือข้อมูลหนังสือจาก AnyFlip ได้"
	case strings.Contains(reason, "permission denied"),
		strings.Contains(reason, "no space left on device"),
		strings.Contains(reason, "create temporary workspace"),
		strings.Contains(reason, "create pdf staging folder"):
		return "storage_error", "ระบบไม่สามารถเขียนไฟล์ชั่วคราวหรือ PDF ได้ กรุณาตรวจสอบพื้นที่และสิทธิ์ของโฟลเดอร์บนเซิร์ฟเวอร์"
	case strings.Contains(reason, "x509:"), strings.Contains(reason, "certificate verify"):
		return "tls_error", "ตรวจสอบใบรับรอง HTTPS ของเว็บไซต์ต้นทางไม่สำเร็จ"
	case strings.Contains(reason, "resolve anyflip hostname"),
		strings.Contains(reason, "cannot connect to public anyflip address"),
		strings.Contains(reason, "no such host"):
		return "network_error", "เซิร์ฟเวอร์ไม่สามารถเชื่อมต่อ AnyFlip ได้ กรุณาตรวจสอบ DNS และการเชื่อมต่อขาออก"
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "network_error", "เซิร์ฟเวอร์ไม่สามารถแก้ชื่อโดเมน AnyFlip ได้"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "network_timeout", "การเชื่อมต่อกับเว็บไซต์ต้นทางหมดเวลา กรุณาลองใหม่"
	}
	return "job_failed", "ไม่สามารถดาวน์โหลดหรือสร้าง PDF ได้ กรุณาตรวจสอบบันทึกของเซิร์ฟเวอร์โดยใช้ Job ID"
}
