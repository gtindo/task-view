# Installs task-view for the current user on Windows, without administrator
# rights: downloads the release binary, checks it against the release's
# SHA256SUMS, puts it in %LOCALAPPDATA%\Programs\task-view and adds that
# folder to the user PATH when needed. Run it again to update.
#
#   irm https://raw.githubusercontent.com/gtindo/task-view/main/scripts/install.ps1 | iex
#
# Environment:
#   TASK_VIEW_VERSION      release tag to install, e.g. v1.0.0 (default: latest)
#   TASK_VIEW_INSTALL_DIR  where to put the binary
#
# The script block keeps its settings and variables out of the caller's
# session, which matters when it runs through iex.
& {
  $ErrorActionPreference = "Stop"
  $ProgressPreference = "SilentlyContinue"
  # Windows PowerShell 5.1 may default to TLS 1.0, which GitHub refuses.
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

  $version = if ($env:TASK_VIEW_VERSION) { $env:TASK_VIEW_VERSION } else { "latest" }
  $dir = if ($env:TASK_VIEW_INSTALL_DIR) { $env:TASK_VIEW_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\task-view" }
  $file = "task-view-windows-amd64.exe"
  $repo = "https://github.com/gtindo/task-view/releases"
  $base = if ($version -eq "latest") { "$repo/latest/download" } else { "$repo/download/$version" }

  $tmp = Join-Path $env:TEMP $file
  $sums = Join-Path $env:TEMP "task-view-SHA256SUMS"
  Write-Host "Downloading $file ($version)..."
  Invoke-WebRequest "$base/$file" -OutFile $tmp -UseBasicParsing
  Invoke-WebRequest "$base/SHA256SUMS" -OutFile $sums -UseBasicParsing

  $expected = (Select-String -Path $sums -Pattern " $file$").Line.Split(" ")[0]
  $actual = (Get-FileHash $tmp -Algorithm SHA256).Hash
  Remove-Item $sums
  if ($actual -ne $expected) {
    Remove-Item $tmp
    throw "Checksum mismatch for $file"
  }

  New-Item -ItemType Directory -Force $dir | Out-Null
  $exe = Join-Path $dir "task-view.exe"
  Move-Item -Force $tmp $exe

  # Clears the "downloaded from the internet" mark, Windows' counterpart of
  # the macOS quarantine flag.
  Unblock-File $exe
  & $exe version

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if (($userPath -split ";") -contains $dir) {
    Write-Host "Installed $exe"
    return
  }

  # Only the user PATH is extended; copying $env:Path here would duplicate
  # the machine PATH into it.
  $newPath = if ($userPath) { "$userPath;$dir" } else { $dir }
  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
  Write-Host "Installed $exe and added $dir to your PATH. Open a new terminal."
}
