# Instala a Fonteca com Docker no Windows (PowerShell 5.1 ou 7).
# Faz o mesmo que scripts/instalar.sh: cria o .env com senhas aleatorias,
# sobe o banco, aplica schema e papeis, carrega municipios, sobe API e tela e
# emite a primeira chave de API.
#
#   powershell -ExecutionPolicy Bypass -File scripts\instalar.ps1
#   $env:DOMINIO = 'fonteca.seudominio.com.br'; .\scripts\instalar.ps1
#
# Pode rodar de novo: nao troca senhas ja criadas nem emite chave nova se ja
# existir uma conta.
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

# Texto enviado para comandos externos (psql) em UTF-8, sem BOM.
$OutputEncoding = New-Object System.Text.UTF8Encoding($false)
[Console]::OutputEncoding = $OutputEncoding

function Passo($texto) { Write-Host ''; Write-Host "==> $texto" -ForegroundColor Cyan }
function Falhar($texto) { Write-Host "erro: $texto" -ForegroundColor Red; exit 1 }

# Para em qualquer comando externo que falhe (o PowerShell 5.1 nao faz isso sozinho).
function Rodar {
  & $args[0] $args[1..($args.Length - 1)]
  if ($LASTEXITCODE -ne 0) { Falhar "falhou: $($args -join ' ')" }
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
  Falhar 'Docker nao encontrado. Instale o Docker Desktop: https://docs.docker.com/desktop/'
}
docker compose version *> $null
if ($LASTEXITCODE -ne 0) { Falhar "o plugin 'docker compose' nao foi encontrado (Docker Compose v2)." }

function Nova-Senha {
  $bytes = New-Object byte[] 24
  [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
  ($bytes | ForEach-Object { $_.ToString('x2') }) -join ''
}

$arquivoEnv = Join-Path (Get-Location) '.env'
$utf8 = New-Object System.Text.UTF8Encoding($false)
function Garantir($nome, $valor) {
  $atual = if (Test-Path $arquivoEnv) { [IO.File]::ReadAllText($arquivoEnv) } else { '' }
  if ($atual -notmatch "(?m)^$nome=") {
    [IO.File]::AppendAllText($arquivoEnv, "$nome=$valor`n", $utf8)
  }
}

Passo 'Configuracao (.env)'
$dominio = if ($env:DOMINIO) { $env:DOMINIO } else { '' }
$portaHttp = if ($env:PORTA_HTTP) { $env:PORTA_HTTP } elseif ($dominio) { '80' } else { '8080' }
$portaHttps = if ($env:PORTA_HTTPS) { $env:PORTA_HTTPS } elseif ($dominio) { '443' } else { '8443' }
Garantir 'POSTGRES_PASSWORD' (Nova-Senha)
Garantir 'FONTECA_API_SENHA' (Nova-Senha)
Garantir 'FONTECA_INGEST_SENHA' (Nova-Senha)
Garantir 'FONTECA_LEITURA_SENHA' (Nova-Senha)
Garantir 'DOMINIO' $dominio
Garantir 'PORTA_HTTP' $portaHttp
Garantir 'PORTA_HTTPS' $portaHttps
Garantir 'PG_SHARED_BUFFERS' '512MB'

$cfg = @{}
foreach ($linha in [IO.File]::ReadAllLines($arquivoEnv)) {
  if ($linha -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') { $cfg[$Matches[1]] = $Matches[2] }
}
Write-Host "ok ($($cfg.Count) variaveis em .env)"

Passo 'Construindo as imagens (a primeira vez leva alguns minutos)'
Rodar docker compose build

Passo 'Subindo o banco'
Rodar docker compose up -d --wait db

function Psql { docker compose exec -T db psql -v ON_ERROR_STOP=1 -q -U fonteca -d fonteca @args }

Passo 'Aplicando o schema'
Rodar docker compose run --rm cli migrate

Passo 'Criando os papeis do banco (a API roda com permissao minima)'
Get-Content 'deploy\papeis.sql' -Raw | Psql
if ($LASTEXITCODE -ne 0) { Falhar 'nao foi possivel aplicar deploy/papeis.sql' }
Psql -c "ALTER ROLE fonteca_api PASSWORD '$($cfg.FONTECA_API_SENHA)'" `
     -c "ALTER ROLE fonteca_ingest PASSWORD '$($cfg.FONTECA_INGEST_SENHA)'" `
     -c "ALTER ROLE fonteca_leitura PASSWORD '$($cfg.FONTECA_LEITURA_SENHA)'"
if ($LASTEXITCODE -ne 0) { Falhar 'nao foi possivel definir as senhas dos papeis' }

Passo 'Carregando o de-para de municipios (Tesouro Transparente)'
docker compose run --rm cli ibge
if ($LASTEXITCODE -ne 0) { Write-Host 'aviso: nao foi possivel baixar agora; rode depois: docker compose run --rm cli ibge' -ForegroundColor Yellow }

Passo 'Subindo a API e a tela'
Rodar docker compose up -d --wait api web

Passo 'Conta e chave de API'
$chave = ''
$contas = (Psql -tAc 'SELECT count(*) FROM meta.conta' | Out-String).Trim()
if ($contas -eq '0') {
  $nomeConta = if ($env:CONTA) { $env:CONTA } else { 'Principal' }
  $saida = docker compose run --rm cli conta -nome $nomeConta | Out-String
  if ($saida -match '(?m)^chave:\s*(\S+)') { $chave = $Matches[1] }
} else {
  Write-Host 'ja existe conta; nenhuma chave nova emitida (para outra: docker compose run --rm cli conta -nome "Nome")'
}

$url = if ($dominio) { "https://$dominio" } else { "http://localhost:$($cfg.PORTA_HTTP)" }

Write-Host ''
Write-Host 'Fonteca instalada.' -ForegroundColor Green
Write-Host ''
Write-Host "  Tela:  $url"
Write-Host "  API:   $url/v1/health"
if ($chave) {
  Write-Host ''
  Write-Host '  Sua chave de API (guarde agora; so o hash fica no banco):'
  Write-Host ''
  Write-Host "    $chave" -ForegroundColor Yellow
}
Write-Host @'

Proximo passo: carregar os dados da Receita Federal. Baixa ~7 GB do portal
de dados abertos; o download retoma se cair.

  So alguns ramos (minutos para carregar, pouco disco):
    docker compose run --rm cli ingest -cnae 5611201,5611203

  Base completa (horas para carregar, ~30 GB de disco):
    docker compose run --rm cli ingest

Enquanto isso, a tela ja funciona no modo Demonstracao.
'@
