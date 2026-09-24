<p align="center">
  <img src="web/public/marca/simbolo.png" alt="" width="72" />
</p>

<h1 align="center">Fonteca</h1>

<p align="center"><strong>Dados oficiais. Decisões melhores.</strong></p>

<p align="center">
  Encontre as empresas que acabaram de abrir no seu mercado, direto do cadastro
  público da Receita Federal, na sua própria máquina.
</p>

<p align="center">
  <img src="docs/imagens/radar.png" alt="Radar da Fonteca: mapa do Brasil com os estados acesos pela quantidade de empresas encontradas" width="820" />
</p>

---

Toda empresa aberta no Brasil entra no Cadastro Nacional da Pessoa Jurídica,
que a Receita Federal publica todo mês como **dado aberto**. O dado é gratuito,
mas vem em dezenas de arquivos brutos, sem índice, impossíveis de consultar
direto. Os serviços que organizam esse cadastro cobram por busca.

A Fonteca faz esse trabalho na sua máquina ou na sua VM: baixa os arquivos,
organiza num Postgres e entrega uma tela e uma API para você achar quem acabou
de abrir no seu ramo, com celular para chamar no WhatsApp.

## O que ela faz

- **Busca por ramo.** As 1.332 atividades oficiais da CNAE (IBGE), com busca por
  texto e por termos populares: "padaria", "oficina", "salão de beleza".
- **Radar por estado.** O mapa do Brasil mostra onde estão as empresas do
  filtro; clique no estado para filtrar.
- **Filtros de prospecção.** Abertas nos últimos 30, 90, 180 ou 365 dias,
  situação cadastral, só com celular, sem MEI.
- **Pronto para agir.** Botão de WhatsApp com o número já montado (a Receita
  guarda o celular sem o nono dígito; a Fonteca restaura), e exportação para
  planilha que o Excel em português abre direto.
- **API REST** com chave, limite de uso e isolamento entre contas, para ligar
  no seu CRM ou automação.
- **Modo demonstração**, com empresas fictícias, para conhecer a ferramenta
  antes de baixar os dados.

Nada sai da sua máquina: não há conta, cadastro nem servidor de terceiros.

## Instalar

Precisa só do [Docker](https://docs.docker.com/get-docker/).

```bash
git clone https://github.com/magoautomacoes/fonteca.git
cd fonteca
scripts/instalar.sh                 # Linux, macOS ou VM
```

No Windows, no PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\instalar.ps1
```

O instalador cria as senhas, prepara o banco, sobe a tela em
**http://localhost:8080** e mostra a sua chave de API. Depois, carregue os
dados da Receita:

```bash
# só os ramos que interessam: minutos para carregar, pouco disco
docker compose run --rm cli ingest -cnae 5611201,5611203

# ou a base inteira: algumas horas, cerca de 30 GB
docker compose run --rm cli ingest
```

O passo a passo completo, incluindo VM com HTTPS, requisitos de máquina e
atualização mensal, está em **[docs/instalacao.md](docs/instalacao.md)**.

## Documentação

| | |
|---|---|
| [Instalação](docs/instalacao.md) | Máquina local, VM com domínio e HTTPS, atualização, backup |
| [API](docs/api.md) | Rotas, filtros, exemplos com `curl` |
| [Uso responsável](docs/uso-responsavel.md) | LGPD, WhatsApp e boas práticas de prospecção |
| [Arquitetura](docs/arquitetura.md) | Como os dados são organizados e as decisões por trás |
| [Desenvolvimento](docs/desenvolvimento.md) | Rodar do código-fonte, testes, contribuir |

## Uso responsável

O cadastro é público, mas o uso de dados pessoais é regulado pela LGPD: vale
para o celular e o e-mail de empresário individual, por exemplo. Use a Fonteca
para abordagem comercial pontual e relevante, atenda quem pedir para não ser
contatado e não faça disparo em massa (o WhatsApp bane números que fazem isso).
Leia **[docs/uso-responsavel.md](docs/uso-responsavel.md)**.

A Fonteca não tem vínculo com a Receita Federal. Os dados são os publicados
pela Receita, sem garantia de que estejam atualizados ou corretos.

## Apoie

A Fonteca é gratuita e continua gratuita. Se ela economizou seu tempo ou seu
dinheiro, uma contribuição ajuda a manter o projeto:

<!-- TODO: chave Pix e/ou endereço Bitcoin -->
*Em breve.*

## Licença

[MIT](LICENSE). Use, modifique e hospede à vontade, mantendo o aviso de autoria.

Fontes dos dados: Cadastro Nacional da Pessoa Jurídica (Receita Federal),
Classificação Nacional de Atividades Econômicas 2.3 e malha de UFs (IBGE),
tabela de municípios do SIAFI (Tesouro Transparente).
