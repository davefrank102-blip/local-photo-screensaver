' Fetch first page on main thread only for host; scene loads playlist so status is visible.
function loadServerHost() as string
  sec = createObject("roRegistrySection", "LocalPhotoSpike")
  if sec.exists("serverHost") then return sec.read("serverHost")
  return "192.168.1.10"
end function

sub Main()
  screen = createObject("roSGScreen")
  port = createObject("roMessagePort")
  screen.setMessagePort(port)
  scene = screen.createScene("SettingsScene")
  screen.show()
  while true
    msg = wait(0, port)
    if type(msg) = "roSGScreenEvent"
      if msg.isScreenClosed() then return
    end if
  end while
end sub

sub RunScreenSaver()
  host = loadServerHost()
  baseUrl = "http://" + host + ":8787"
  screen = createObject("roSGScreen")
  port = createObject("roMessagePort")
  screen.setMessagePort(port)
  globals = screen.getGlobalNode()
  globals.addFields({ serverHost: host, serverBase: baseUrl })
  scene = screen.createScene("ScreensaverScene")
  screen.show()
  while true
    msg = wait(0, port)
    if type(msg) = "roSGScreenEvent"
      if msg.isScreenClosed() then return
    end if
  end while
end sub

sub RunScreenSaverSettings()
  screen = createObject("roSGScreen")
  port = createObject("roMessagePort")
  screen.setMessagePort(port)
  scene = screen.createScene("SettingsScene")
  screen.show()
  while true
    msg = wait(0, port)
    if type(msg) = "roSGScreenEvent"
      if msg.isScreenClosed() then return
    end if
  end while
end sub
