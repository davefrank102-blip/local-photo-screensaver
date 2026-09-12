' Local Photo Screensaver — Phase 0
' Screensaver packages must expose RunScreenSaver(); do NOT define RunUserInterface/Main channel UI.

sub RunScreenSaver()
  screen = createObject("roSGScreen")
  port = createObject("roMessagePort")
  screen.setMessagePort(port)
  scene = screen.createScene("ScreensaverScene")
  screen.show()
  ' Keep the screensaver alive until the platform dismisses it
  while true
    msg = wait(0, port)
    msgType = type(msg)
    if msgType = "roSGScreenEvent"
      if msg.isScreenClosed() then return
    end if
  end while
end sub

' Optional settings entry: store LAN host IP in registry for the spike.
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
