# Instalação

A Fonteca roda em Docker: um Postgres, a API e a tela (servida pelo Caddy,
que também faz o HTTPS). O instalador cuida de tudo que é repetitivo.

## Requisitos

| | Só alguns ramos (`-cnae`) | Base completa |
|---|---|---|
| Memória | 4 GB | 8 GB (16 GB recomendado) |
| Disco | 10 GB | 40 GB |
| Tempo de carga | minutos | algumas horas |

Nos dois casos o download da Receita é de cerca de 7 GB, porque ela não separa
os arquivos por atividade. Os ZIPs ficam guardados para o download retomar se
cair; depois da carga dá para apagá-los (veja "Espaço em disco").

Sistema: Linux, macOS ou Windows com [Docker](https://docs.docker.com/get-docker/)
e o Docker Compose v2 (já vem com o Docker Desktop).

## Na sua máquina

```bash
git clone https://github.com/magoautomacoes/fonteca.git
cd fonteca
scripts/instalar.sh
```

No Windows, no PowerShell, a partir da pasta do projeto:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\instalar.ps1
```

O instalador:

1. cria o `.env` com senhas aleatórias (nunca vai para o git);
2. constrói as imagens e sobe o banco;
3. aplica o schema e cria os papéis do banco, com a API rodando com permissão
   mínima;
4. carrega o de-para de municípios do Tesouro Transparente;
5. sobe a API e a tela em **http://localhost:8080**;
6. cria a primeira conta e mostra a **chave de API**. Guarde: só o hash fica
   no banco.

Pode rodar de novo quando quiser: ele não troca senhas nem emite chave nova se
já houver uma conta.

Para usar outra porta: `PORTA_HTTP=9000 scripts/instalar.sh` (ou edite
`PORTA_HTTP` no `.env` e rode `docker compose up -d`).

## Carregar os dados da Receita

```bash
# só alguns ramos: restaurantes e lanchonetes, por exemplo
docker compose run --rm cli ingest -cnae 5611201,5611203

# incluir quem tem o ramo como atividade secundária (dobra o alcance)
docker compose run --rm cli ingest -cnae 5611201,5611203 -cnae-secundario

# a base inteira
docker compose run --rm cli ingest

# ver o que seria baixado, sem baixar
docker compose run --rm cli ingest -dry-run
```

Os códigos CNAE estão na própria tela: busque o ramo e veja o código de cada
atividade. A carga cria um lote novo e só troca o vigente no fim: durante a
carga a tela continua respondendo com o lote anterior.

## Numa VM, com domínio e HTTPS

1. Crie a VM (Ubuntu 24.04, por exemplo) e instale o Docker:
   `curl -fsSL https://get.docker.com | sh`.
2. Aponte um registro DNS (`fonteca.seudominio.com.br`) para o IP da VM.
3. Libere as portas **80** e **443** no firewall da VM e do provedor.
4. Instale com o domínio:

   ```bash
   git clone https://github.com/magoautomacoes/fonteca.git && cd fonteca
   DOMINIO=fonteca.seudominio.com.br scripts/instalar.sh
   ```

O Caddy emite e renova o certificado sozinho (Let's Encrypt). O banco não fica
exposto: só a API e a tela, pelo Caddy.

A tela pede a chave de API para mostrar dados reais, então quem acessa sem a
chave vê só o modo demonstração. Para dar acesso a outra pessoa, emita uma
chave para ela:

```bash
docker compose run --rm cli conta -nome "Maria"
```

## Atualização mensal

A Receita publica um lote novo todo mês. O `scripts/atualizar.sh` carrega o
mais recente e sai sem fazer nada se ele já estiver carregado, então pode rodar
todo dia no cron:

```bash
crontab -e
# todo dia às 4h, só os ramos que você usa
0 4 * * * /caminho/da/fonteca/scripts/atualizar.sh -cnae 5611201,5611203 >> /var/log/fonteca.log 2>&1
```

## Comandos úteis

```bash
docker compose ps                                  # o que está rodando
docker compose logs -f api                         # logs da API
docker compose run --rm cli conta -nome "Nome"     # outra chave de API
docker compose run --rm cli export -cnae 5611201 -uf SP -ultimos-dias 90 -so-celular > leads.csv
docker compose down                                # para tudo (os dados ficam)
docker compose exec db psql -U fonteca -d fonteca  # SQL direto
```

Para consultas SQL prontas, veja [consultas-uteis.sql](consultas-uteis.sql).

## Atualizar a Fonteca

```bash
git pull
docker compose build
docker compose run --rm cli migrate
docker compose up -d
```

## Backup

O que não se recupera baixando de novo da Receita são as contas, as chaves e o
histórico de uso, no schema `meta`:

```bash
docker compose exec -T db pg_dump -U fonteca -d fonteca -n meta > fonteca-meta.sql
```

Os lotes da Receita dá para recarregar a qualquer momento com `ingest`.

## Espaço em disco

Depois que um lote carregou, os ZIPs só servem para retomar um download que
caiu. Para liberar os ~7 GB:

```bash
docker compose run --rm --entrypoint sh cli -c 'rm -rf /var/lib/fonteca/lotes/*'
```

## Desinstalar

```bash
docker compose down -v   # -v apaga também o banco e os ZIPs
```
