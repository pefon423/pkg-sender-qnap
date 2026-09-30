#Requires -Version 5.1
# Registers a Scheduled Task that runs the PS5 PKG Sender when the current
# user logs in. Does not require an elevated PowerShell (unlike an
# AtStartup/SYSTEM-context task, which Register-ScheduledTask refuses
# without admin rights).

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$runScript = Join-Path $scriptDir "run.ps1"
$taskName = "PKGSenderNAS"

$action = New-ScheduledTaskAction -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$runScript`""

$trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"

$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit (New-TimeSpan -Days 0)

Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger `
    -Settings $settings -Force `
    -Description "PS5 PKG Sender - serves PKG files to a PS5 running pkg-receiver.elf over the LAN."

Write-Output "Registered scheduled task '$taskName'. Starting it now..."
Start-ScheduledTask -TaskName $taskName
Start-Sleep -Seconds 2
Get-ScheduledTask -TaskName $taskName | Get-ScheduledTaskInfo
