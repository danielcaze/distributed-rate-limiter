$ErrorActionPreference = 'Stop'

$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$tools = Join-Path $root '.tools'
$archive = Join-Path $tools 'protoc-29.3-win64.zip'
$compilerDir = Join-Path $tools 'protoc-29.3'
$compiler = Join-Path $compilerDir 'bin/protoc.exe'
$bin = Join-Path $tools 'bin'
$url = 'https://github.com/protocolbuffers/protobuf/releases/download/v29.3/protoc-29.3-win64.zip'
$sha256 = '57EA59E9F551AD8D71FFAA9B5CFBE0CA1F4E720972A1DB7EC2D12AB44BFF9383'

New-Item -ItemType Directory -Force -Path $tools, $bin | Out-Null
if (-not (Test-Path $archive)) {
    Invoke-WebRequest -Uri $url -OutFile $archive
}
if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $sha256) {
    throw "protoc archive checksum mismatch: $archive"
}
if (-not (Test-Path $compiler)) {
    Expand-Archive -Path $archive -DestinationPath $compilerDir
}
if ((& $compiler --version) -ne 'libprotoc 29.3') {
    throw 'Unexpected protoc version'
}

$previousGobin = $env:GOBIN
try {
    $env:GOBIN = $bin
    & go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6
    if ($LASTEXITCODE -ne 0) { throw 'protoc-gen-go installation failed' }
    & go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
    if ($LASTEXITCODE -ne 0) { throw 'protoc-gen-go-grpc installation failed' }
} finally {
    $env:GOBIN = $previousGobin
}

Push-Location $root
try {
    $arguments = @(
        '--proto_path=proto'
        "--proto_path=$compilerDir/include"
        "--plugin=protoc-gen-go=$bin/protoc-gen-go.exe"
        "--plugin=protoc-gen-go-grpc=$bin/protoc-gen-go-grpc.exe"
        '--go_out=.'
        '--go_opt=module=github.com/danielcaze/distributed-rate-limiter'
        '--go-grpc_out=.'
        '--go-grpc_opt=module=github.com/danielcaze/distributed-rate-limiter'
        'limiter/v1/limiter.proto'
    )
    & $compiler @arguments
    if ($LASTEXITCODE -ne 0) { throw 'protoc generation failed' }
} finally {
    Pop-Location
}
