import { GenerateThumbnail } from '../../wailsjs/go/main/App'

// Hàng đợi trích ảnh xem trước cho thẻ clip.
//
// Vì sao cần: trước đây thẻ clip nào chưa có ảnh thì render thẳng
// <video preload="auto"> trỏ vào FILE GỐC. Với video 1 tiếng và 40 thẻ trên lưới,
// WebView2 phải dựng 40 bộ giải mã cùng lúc, mỗi cái seek + buffer trên file vài GB
// → ngốn ~5 GB RAM, renderer treo (WebView2 báo kind 2) rồi chết (kind 0).
// Giờ mỗi thẻ chỉ xin 1 ảnh JPG do ffmpeg trích ở backend: nhẹ hơn vài trăm lần.
//
// Ba lớp bảo vệ:
//  1. Cache theo (đường dẫn + mốc giây) — cuộn qua cuộn lại không gọi lại ffmpeg.
//  2. Gộp lời gọi trùng — 2 thẻ cùng mốc chỉ chạy 1 tiến trình.
//  3. Chặn trần số tiến trình ffmpeg chạy song song, tránh 40 ffmpeg cùng lúc
//     làm máy khựng đúng lúc người dùng đang cuộn.

const MAX_PARALLEL = 3

const cache = new Map<string, string>()
const inflight = new Map<string, Promise<string>>()
const waiting: Array<() => void> = []
let running = 0

const keyOf = (path: string, timeSec: number) => `${path}|${timeSec.toFixed(3)}`

const acquireSlot = (): Promise<void> => {
  if (running < MAX_PARALLEL) {
    running++
    return Promise.resolve()
  }
  return new Promise<void>((resolve) => {
    waiting.push(() => {
      running++
      resolve()
    })
  })
}

const releaseSlot = () => {
  running--
  const next = waiting.shift()
  if (next) next()
}

/**
 * Lấy ảnh xem trước tại mốc timeSec của video path. Trả về đường dẫn ảnh trên đĩa
 * (chuỗi rỗng nếu ffmpeg không trích được — thẻ sẽ hiện ô giữ chỗ).
 */
export const requestThumb = (path: string, timeSec: number): Promise<string> => {
  if (!path) return Promise.resolve('')
  const key = keyOf(path, timeSec)

  const hit = cache.get(key)
  if (hit !== undefined) return Promise.resolve(hit)

  const pending = inflight.get(key)
  if (pending) return pending

  const task = (async () => {
    await acquireSlot()
    try {
      const out = await GenerateThumbnail(path, timeSec)
      cache.set(key, out || '')
      return out || ''
    } catch (e) {
      // Ghi nhớ cả thất bại để không thử lại vô hạn mỗi lần thẻ vào tầm nhìn.
      cache.set(key, '')
      return ''
    } finally {
      releaseSlot()
      inflight.delete(key)
    }
  })()

  inflight.set(key, task)
  return task
}
