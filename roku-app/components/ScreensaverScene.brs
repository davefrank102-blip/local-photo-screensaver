' Phase 0 SceneGraph screensaver: fetch spike playlist, cross-fade stills, prefetch next.

sub init()
  m.posterA = m.top.findNode("posterA")
  m.posterB = m.top.findNode("posterB")
  m.status = m.top.findNode("statusLabel")
  m.fadeToA = m.top.findNode("fadeToA")
  m.fadeToB = m.top.findNode("fadeToB")
  m.activeIsA = true
  m.items = []
  m.index = 0
  m.displaySeconds = 8
  m.baseUrl = ""
  m.timer = createObject("roSGNode", "Timer")
  m.timer.repeat = true
  m.timer.observeField("fire", "onSlide")
  startScreensaver()
end sub

sub startScreensaver()
  host = loadServerHost()
  if host = invalid or Len(host) = 0
    m.status.text = "Set server IP in screensaver settings"
    return
  end if
  m.baseUrl = "http://" + host + ":8787"
  m.status.text = "Loading playlist from " + host + "…"
  fetchPlaylist()
end sub

function loadServerHost() as dynamic
  sec = createObject("roRegistrySection", "LocalPhotoSpike")
  if sec.exists("serverHost")
    return sec.read("serverHost")
  end if
  ' TODO Phase 0 fallback — change via RunScreenSaverSettings
  return "192.168.1.10"
end function

sub fetchPlaylist()
  url = m.baseUrl + "/api/v1/spike/playlist"
  xfer = createObject("roUrlTransfer")
  xfer.SetUrl(url)
  xfer.RetainBodyOnError(true)
  body = xfer.GetToString()
  if body = invalid or Len(body) = 0
    m.status.text = "Playlist fetch failed — check server & IP"
    return
  end if
  parsed = ParseJson(body)
  if parsed = invalid or parsed.items = invalid or parsed.items.Count() = 0
    m.status.text = "No photos in playlist"
    return
  end if
  if parsed.displaySeconds <> invalid
    m.displaySeconds = parsed.displaySeconds
  end if
  m.items = parsed.items
  m.index = 0
  m.status.text = ""
  ' First image on poster A
  m.posterA.uri = absoluteUrl(m.items[0].imageUrl)
  m.posterA.opacity = 1.0
  m.posterB.opacity = 0.0
  m.activeIsA = true
  prefetchNext()
  m.timer.duration = m.displaySeconds
  m.timer.control = "start"
end sub

sub onSlide()
  if m.items.Count() = 0 then return
  m.index = (m.index + 1) mod m.items.Count()
  nextUrl = absoluteUrl(m.items[m.index].imageUrl)
  if m.activeIsA
    m.posterB.uri = nextUrl
    m.fadeToB.control = "start"
    m.activeIsA = false
  else
    m.posterA.uri = nextUrl
    m.fadeToA.control = "start"
    m.activeIsA = true
  end if
  prefetchNext()
end sub

sub prefetchNext()
  if m.items.Count() < 2 then return
  ' Warm the inactive poster with index+1 (will be overwritten on slide if needed)
  nextIdx = (m.index + 1) mod m.items.Count()
  url = absoluteUrl(m.items[nextIdx].imageUrl)
  if m.activeIsA
    if m.posterB.uri <> url then m.posterB.uri = url
  else
    if m.posterA.uri <> url then m.posterA.uri = url
  end if
end sub

function absoluteUrl(path as string) as string
  if Left(path, 4) = "http" then return path
  if Left(path, 1) = "/" then return m.baseUrl + path
  return m.baseUrl + "/" + path
end function
