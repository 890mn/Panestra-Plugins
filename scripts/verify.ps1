$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
$goBinary=if($env:PANESTRA_GO){$env:PANESTRA_GO}else{(Get-Command go).Source}
$gofmt=Join-Path (Split-Path $goBinary) 'gofmt.exe'
$unformatted=& $gofmt -l (rg --files -g '*.go' -g '!artifacts/**')
if($unformatted){throw "Go formatting failed: $unformatted"}
& $goBinary vet ./...
if($LASTEXITCODE-ne 0){throw 'Go vet failed'}
& $goBinary test ./... -count=1
if($LASTEXITCODE-ne 0){throw 'Plugin tests failed'}
$python=if($env:PANESTRA_PYTHON){$env:PANESTRA_PYTHON}else{(Get-Command python).Source}
& $python tests/test-alas-bridge.py
if($LASTEXITCODE-ne 0){throw 'ALAS bridge tests failed'}

$nodeBinary=if($env:PANESTRA_NODE){$env:PANESTRA_NODE}else{(Get-Command node).Source}
& $nodeBinary --test tests/netease-timeline.test.mjs
if($LASTEXITCODE-ne 0){throw 'NetEase timeline tests failed'}
