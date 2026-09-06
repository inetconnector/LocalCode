' SPDX-License-Identifier: Apache-2.0
Set WshShell = CreateObject("WScript.Shell")
Set FSO = CreateObject("Scripting.FileSystemObject")
scriptDir = FSO.GetParentFolderName(WScript.ScriptFullName)
WshShell.CurrentDirectory = scriptDir

exePath = FSO.BuildPath(scriptDir, "dist\LocalCode.exe")
If Not FSO.FileExists(exePath) Then
    WshShell.Run "cmd.exe /c """ & FSO.BuildPath(scriptDir, "BUILD.bat") & """", 1, True
End If

WshShell.Environment("PROCESS")("LOCALCODE_FAST_START") = "1"
WshShell.Environment("PROCESS")("LOCALCODE_SUPPRESS_FATAL_DIALOGS") = "1"
WshShell.Run "taskkill /F /IM LocalCode.exe", 0, True
WshShell.Run """" & exePath & """", 1, False
