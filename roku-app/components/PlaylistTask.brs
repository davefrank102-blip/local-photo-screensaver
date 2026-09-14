sub init()
  m.top.functionName = "fetchPlaylist"
end sub

sub fetchPlaylist()
  m.top.error = ""
  m.top.response = ""
  url = m.top.requestUrl
  if url = invalid or Len(url) = 0
    m.top.error = "No playlist URL"
    return
  end if
  xfer = createObject("roUrlTransfer")
  port = createObject("roMessagePort")
  xfer.SetMessagePort(port)
  xfer.SetUrl(url)
  xfer.RetainBodyOnError(true)
  xfer.EnableEncodings(true)
  ok = xfer.AsyncGetToString()
  if not ok
    m.top.error = "Could not start request"
    return
  end if
  msg = wait(12000, port)
  if msg = invalid
    m.top.error = "Timed out"
    return
  end if
  if type(msg) <> "roUrlEvent"
    m.top.error = "Bad network event"
    return
  end if
  code = msg.GetResponseCode()
  body = msg.GetString()
  if code < 200 or code >= 300 or body = invalid or Len(body) = 0
    m.top.error = "HTTP " + Str(code).Trim()
    return
  end if
  m.top.response = body
end sub
