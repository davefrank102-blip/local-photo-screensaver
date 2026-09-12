sub init()
  m.kb = m.top.findNode("kb")
  m.current = m.top.findNode("current")
  host = readHost()
  m.current.text = "Current: " + host
  m.kb.text = host
  m.kb.setFocus(true)
  m.kb.observeField("text", "onText")
  ' Persist when user finishes typing (OK on keyboard)
  m.top.observeField("focusedChild", "noop")
end sub

sub noop()
end sub

sub onText()
  text = m.kb.text
  if text <> invalid and text <> ""
    writeHost(text)
    m.current.text = "Saved: " + text + "  (press Back when done)"
  end if
end sub

function readHost() as string
  sec = createObject("roRegistrySection", "LocalPhotoSpike")
  if sec.exists("serverHost")
    return sec.read("serverHost")
  end if
  return "192.168.1.10"
end function

sub writeHost(host as string)
  sec = createObject("roRegistrySection", "LocalPhotoSpike")
  sec.write("serverHost", host)
  sec.flush()
end sub
