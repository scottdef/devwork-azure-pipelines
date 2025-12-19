# Prerequisites installation for Windows 11
Write-Host "=== Installing Prerequisites ===" -ForegroundColor Cyan
$UserBinDir = "$env:USERPROFILE\bin"
if (!(Test-Path $UserBinDir)) { New-Item -ItemType Directory -Path $UserBinDir -Force | Out-Null }
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$UserBinDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$currentPath;$UserBinDir", "User")
    $env:Path = "$env:Path;$UserBinDir"
}
if (!(Get-Command az -ErrorAction SilentlyContinue)) {
    Write-Host "Install Azure CLI from: https://aka.ms/installazurecliwindows" -ForegroundColor Yellow
}
az aks install-cli --install-location "$UserBinDir\kubectl.exe" --kubelogin-install-location "$UserBinDir\kubelogin.exe"
$helmUrl = "https://get.helm.sh/helm-v3.13.3-windows-amd64.zip"
Invoke-WebRequest -Uri $helmUrl -OutFile "$env:TEMP\helm.zip"
Expand-Archive -Path "$env:TEMP\helm.zip" -DestinationPath "$env:TEMP\helm" -Force
Copy-Item "$env:TEMP\helm\windows-amd64\helm.exe" -Destination "$UserBinDir\helm.exe" -Force
Write-Host "Prerequisites installed. Restart terminal." -ForegroundColor Green
