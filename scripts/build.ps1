param([string[]]$Plugins=@())
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
$goBinary=if($env:PANESTRA_GO){$env:PANESTRA_GO}else{(Get-Command go).Source}
if (-not $Plugins.Count) { $Plugins=@((Get-Content -LiteralPath catalog.json -Raw|ConvertFrom-Json).plugins | ForEach-Object { ($_.id -split '\.')[ -1 ] }) }
foreach($name in $Plugins){
 if($name -notmatch '^[a-z][a-z0-9-]*$' -or -not (Test-Path -LiteralPath (Join-Path $root "cmd/$name/main.go"))){throw 'Unknown plugin'}
 $destination=Join-Path $root "artifacts/$name"
 New-Item -ItemType Directory -Force $destination|Out-Null
 $manifest=Get-Content -LiteralPath "plugins/$name/manifest.json" -Raw|ConvertFrom-Json
 & $goBinary build -trimpath -ldflags "-X panestra.local/plugins/sdk.Version=$($manifest.version)" -o "$destination/$name-plugin.exe" "./cmd/$name"
 if($LASTEXITCODE-ne 0){throw "Plugin build failed: $name"}
 Copy-Item -LiteralPath "plugins/$name/manifest.json" -Destination "$destination/manifest.json"
}
