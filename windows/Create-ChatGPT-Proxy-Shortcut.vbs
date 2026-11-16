Set shell = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")

launcherDir = fso.GetParentFolderName(WScript.ScriptFullName)
shortcutPath = shell.SpecialFolders("Desktop") & "\ChatGPT Proxy.lnk"
Set shortcut = shell.CreateShortcut(shortcutPath)
shortcut.TargetPath = launcherDir & "\Start-ChatGPT-Proxy.cmd"
shortcut.WorkingDirectory = launcherDir
shortcut.IconLocation = launcherDir & "\chatgpt.ico,0"
shortcut.Description = "Launch ChatGPT Desktop using the configured proxy."
shortcut.Save

MsgBox "ChatGPT Proxy desktop shortcut created.", 64, "ChatGPT Proxy"
