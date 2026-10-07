param([Parameter(Mandatory)][string]$Key,[Parameter(Mandatory)][uint64]$Sequence)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
$goBinary=if($env:PANESTRA_GO){$env:PANESTRA_GO}else{(Get-Command go).Source}
& $goBinary build -trimpath -o artifacts/publisher.exe ./cmd/publisher
if($LASTEXITCODE-ne 0){throw 'Publisher build failed'}
New-Item -ItemType Directory -Force "artifacts/packages/$Sequence"|Out-Null
$catalog=Get-Content -LiteralPath catalog.json -Raw|ConvertFrom-Json
foreach($item in $catalog.plugins){
 $name=($item.id -split '\.')[ -1 ]
 if ($name -notmatch '^[a-z][a-z0-9-]*$') {throw 'Invalid plugin name'}
 $built=Get-Content -LiteralPath "artifacts/$name/manifest.json" -Raw|ConvertFrom-Json
 if($built.id -ne $item.id){throw 'Built plugin identity mismatch'}
 $item.version=$built.version
 & ./artifacts/publisher.exe --key $Key --input "artifacts/$name" --output "artifacts/packages/$Sequence/$name" --sequence $Sequence
 if($LASTEXITCODE-ne 0){throw "Plugin signing failed: $name"}
}
[IO.File]::WriteAllText((Join-Path $root "artifacts/packages/$Sequence/catalog.json"), ($catalog|ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath "$Key.pub" -Destination "artifacts/packages/$Sequence/publisher.pub"
