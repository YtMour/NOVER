// Clipboard access needs a secure context. Console deployments behind plain
// HTTP on a LAN still need copy buttons, so fall back to a temporary textarea.
export async function copyText(value: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return true
    }
  } catch {
    // Fall through to the legacy selection-based copy path.
  }
  try {
    const area = document.createElement('textarea')
    area.value = value
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.top = '0'
    area.style.left = '0'
    area.style.width = '1px'
    area.style.height = '1px'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    const copied = typeof document.execCommand === 'function' ? document.execCommand('copy') : false
    document.body.removeChild(area)
    return copied
  } catch {
    return false
  }
}
