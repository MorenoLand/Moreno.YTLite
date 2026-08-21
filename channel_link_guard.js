(() => {
  if (window === window.top) return
  const nativeOpen = window.open
  function routeChannel(value) {
    if (!value) return false
    let url
    try { url = new URL(String(value), window.location.href) } catch { return false }
    if (!/(^|\.)youtube(?:-nocookie)?\.com$/i.test(url.hostname)) return false
    const match = url.pathname.match(/^\/(channel\/([^/]+)|(@[^/]+)|(?:c|user)\/([^/]+))/i)
    if (!match) return false
    const channelId = match[2] ? decodeURIComponent(match[2]) : ''
    const channel = match[3] ? decodeURIComponent(match[3]) : match[4] ? decodeURIComponent(match[4]) : ''
    window.top.postMessage({ source: 'ytlite', action: 'open-channel', channel, channelId }, '*')
    return true
  }
  window.open = function(value, target, features) {
    if (routeChannel(value)) return null
    return nativeOpen.call(window, value, target, features)
  }
  function intercept(event) {
    const target = event.target
    const anchor = target instanceof Element ? target.closest('a[href]') : null
    if (!anchor || !routeChannel(anchor.href)) return
    event.preventDefault()
    event.stopImmediatePropagation()
  }
  document.addEventListener('click', intercept, true)
  document.addEventListener('auxclick', intercept, true)
})()
