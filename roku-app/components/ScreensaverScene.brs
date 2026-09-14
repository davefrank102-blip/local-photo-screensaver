sub init()
  m.posterA = m.top.findNode("posterA")
  m.posterB = m.top.findNode("posterB")
  m.status = m.top.findNode("statusLabel")
  m.fadeToA = m.top.findNode("fadeToA")
  m.fadeToB = m.top.findNode("fadeToB")
  m.status.text = "Loading..."
  m.activeIsA = true
  m.items = []
  m.index = 0
  m.displaySeconds = 8
  m.baseUrl = "http://192.168.1.10:8787"
  m.seed = "0"
  m.nextOffset = 0
  m.total = 0
  m.loadingPage = false
  m.prefetchingPage = false
  m.queuedItems = invalid
  m.queuedNextOffset = 0
  m.queuedSeed = "0"

  m.timer = createObject("roSGNode", "Timer")
  m.timer.repeat = true
  m.timer.observeField("fire", "onSlide")
  m.top.appendChild(m.timer)

  g = m.global
  if g <> invalid
    if g.serverBase <> invalid and Len(g.serverBase) > 0 then m.baseUrl = g.serverBase
  end if

  m.status.text = "Loading playlist..."
  fetchPage(0, "0", true)
end sub

function numToStr(n) as string
  if n = invalid then return "0"
  tn = type(n)
  if tn = "String" or tn = "roString" then return n
  return Str(n).Trim()
end function

sub fetchPage(offset, seed, isFirst)
  if isFirst
    m.loadingPage = true
  else
    if m.prefetchingPage then return
    if m.queuedItems <> invalid then return
    m.prefetchingPage = true
  end if

  url = m.baseUrl + "/api/v1/spike/playlist?offset=" + numToStr(offset) + "&limit=50"
  if seed <> invalid and Len(seed) > 0 and seed <> "0"
    url = url + "&seed=" + seed
  end if
  m.pendingFirst = isFirst
  m.task = createObject("roSGNode", "PlaylistTask")
  m.top.appendChild(m.task)
  m.task.observeField("response", "onPage")
  m.task.observeField("error", "onPageErr")
  m.task.requestUrl = url
  m.task.control = "RUN"
end sub

sub onPageErr()
  m.loadingPage = false
  m.prefetchingPage = false
  err = ""
  if m.task <> invalid and m.task.error <> invalid then err = m.task.error
  if Len(err) = 0 then err = "Playlist failed"
  if m.pendingFirst then m.status.text = err + " (" + m.baseUrl + ")"
end sub

sub onPage()
  body = ""
  if m.task <> invalid and m.task.response <> invalid then body = m.task.response
  if Len(body) = 0
    m.loadingPage = false
    m.prefetchingPage = false
    if m.pendingFirst then m.status.text = "Empty playlist from " + m.baseUrl
    return
  end if
  parsed = ParseJson(body)
  if parsed = invalid or parsed.items = invalid or parsed.items.Count() = 0
    m.loadingPage = false
    m.prefetchingPage = false
    if m.pendingFirst then m.status.text = "No photos in playlist"
    return
  end if

  if parsed.displaySeconds <> invalid then m.displaySeconds = parsed.displaySeconds

  if m.pendingFirst
    if parsed.seed <> invalid then m.seed = numToStr(parsed.seed)
    if parsed.nextOffset <> invalid then m.nextOffset = parsed.nextOffset
    if parsed.total <> invalid then m.total = parsed.total
    m.items = parsed.items
    m.index = 0
    m.status.text = ""
    m.posterA.uri = absoluteUrl(m.items[0].imageUrl)
    m.posterA.opacity = 1.0
    m.posterB.opacity = 0.0
    m.activeIsA = true
    prefetchNextImage()
    m.timer.duration = m.displaySeconds
    m.timer.control = "start"
    m.loadingPage = false
  else
    ' Background prefetch ? queue for seamless handoff
    m.queuedItems = parsed.items
    if parsed.nextOffset <> invalid then m.queuedNextOffset = parsed.nextOffset
    if parsed.seed <> invalid then m.queuedSeed = numToStr(parsed.seed)
    if parsed.total <> invalid then m.total = parsed.total
    m.prefetchingPage = false
  end if
end sub

sub onSlide()
  if m.loadingPage then return
  if m.items.Count() = 0 then return

  ' Start next batch ~3 slides before the end so EXIF/orient work can finish early
  remainingAfter = m.items.Count() - 1 - m.index
  if remainingAfter <= 3
    fetchPage(m.nextOffset, m.seed, false)
  end if

  if m.index >= m.items.Count() - 1
    if m.queuedItems <> invalid and m.queuedItems.Count() > 0
      promoteQueue()
      return
    end if
    ' Queue not ready yet ? keep showing last slide until prefetch completes
    fetchPage(m.nextOffset, m.seed, false)
    return
  end if

  m.index = m.index + 1
  showCurrent()
  prefetchNextImage()
end sub

sub promoteQueue()
  m.items = m.queuedItems
  m.queuedItems = invalid
  m.nextOffset = m.queuedNextOffset
  if m.queuedSeed <> invalid and Len(m.queuedSeed) > 0 then m.seed = m.queuedSeed
  m.index = 0
  showCurrent()
  prefetchNextImage()
end sub

sub showCurrent()
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
end sub

sub prefetchNextImage()
  if m.items.Count() < 2 then return
  nextIdx = m.index + 1
  if nextIdx >= m.items.Count() then return
  url = absoluteUrl(m.items[nextIdx].imageUrl)
  if m.activeIsA
    if m.posterB.uri <> url then m.posterB.uri = url
  else
    if m.posterA.uri <> url then m.posterA.uri = url
  end if
end sub

function absoluteUrl(path as string) as string
  if path = invalid then return ""
  if Left(path, 4) = "http" then return path
  if Left(path, 1) = "/" then return m.baseUrl + path
  return m.baseUrl + "/" + path
end function
