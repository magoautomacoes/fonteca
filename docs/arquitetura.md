# Arquitetura

Como a Fonteca organiza os dados da Receita Federal e o que cada parte pode
assumir das outras. Leia antes de mexer em `internal/store`.

## Visão geral

```
Receita Federal (WebDAV) ──► ingest ──► lote_AAAA_MM  ◄── api (serve) ◄── web
                                           │                 │
Tesouro Transparente ──► ibge ──► meta ────┘   contas, chaves, uso, lote vigente
```

- **`internal/source`** descobre e baixa os arquivos mensais da Receita
  (WebDAV do portal de dados abertos), com retomada por `Range`.
- **`internal/ingest`** lê os ZIPs em streaming e carrega com `COPY`.
- **`internal/store`** é a única camada que conhece o schema: lote vigente,
  busca, contagem, tipos.
- **`internal/api`** e **`internal/tenant`** servem a API REST: autenticação
  por chave, limites, registro de uso sob RLS.
- **`web/`** é a tela (React + Vite). Não tem endpoint próprio: usa a mesma
  API de qualquer integração.

## Dois schemas: `meta` e o lote

- **`meta`** é permanente: contas, chaves, uso, o lote vigente, DDDs e o
  de-para de municípios. Aplicado por `fonteca migrate`.
- **`lote_AAAA_MM`** é um schema por lote mensal (estabelecimento, empresa,
  simples, municipio, cnae, natureza). Cada ingestão cria um schema novo,
  carrega, indexa e só então troca o vigente em `meta.lote`, numa transação.
  Quem consulta durante a carga continua vendo o lote anterior; o antigo é
  descartado depois da troca.

`DescartarSchema` valida só o formato do nome e **não** protege o lote
vigente. Rotinas de limpeza têm de conferir `LoteVigente` antes.

## Como uma consulta alcança o lote vigente

Não há `search_path`. Trocá-lo com `ALTER DATABASE` no fim da carga só
afetaria conexões novas: conexões já abertas no pool continuariam vendo o lote
antigo, quebrando a troca atômica. Toda consulta descobre o schema e o
qualifica explicitamente:

```go
vigente, err := store.LoteVigente(ctx, pool)   // (nil, nil) = nenhum lote ainda -> 503
sql := fmt.Sprintf(`SELECT ... FROM %s.estabelecimento e WHERE e.cnae_principal = ANY($1) ...`,
    vigente.Schema)
rows, err := pool.Query(ctx, sql, cnaes /* , ... */)
```

O nome do schema é a **única** coisa interpolada em SQL, porque identificador
não pode ser bind parameter. Ele vem de `meta.lote`, gravado só depois de
`validarSchema`; código que monte um nome por conta própria tem de chamar
`store.ValidarSchema` antes. Todo o resto é bind parameter.

`store.Buscar` e `store.ContarPorUF` compartilham o mesmo `WHERE`
(`condicoesDoFiltro`): o mapa do radar conta exatamente as empresas que a
lista devolveria.

## Telefone celular: 8 dígitos começando em 9

`estabelecimento.celular` é coluna gerada: `true` quando o telefone tem **8
dígitos e começa com 9**.

**Não "corrija" isso para 9 dígitos.** A Anatel definiu em 2016 que celular
tem 9 dígitos, mas a **Receita não guarda o nono**. Medido no lote de setembro
de 2026: dos 1.240.332 telefones preenchidos, todos têm 8 dígitos e nenhum tem
9. Exigir 9 dígitos zera a base.

O nono dígito entra só na leitura, ao montar o WhatsApp e ao exibir:

```sql
'55' || e.ddd_1 || '9' || e.telefone_1 AS whatsapp
```

`telefone_1` nulo produz `celular` NULL, não `false`: "não tem telefone" e
"tem telefone fixo" são coisas diferentes. A validação de DDD fica em
`meta.ddd`, corrigível com `UPDATE`, e não na coluna gerada, que exigiria
reimportar o lote para mudar.

## Município: SIAFI, não IBGE

`estabelecimento.municipio` guarda o código **SIAFI**, que é o que a Receita
publica. IBGE, DATASUS e INEP usam o código **IBGE** (São Paulo: SIAFI `7107`,
IBGE `3550308`). Todo join geográfico passa por `meta.municipio_ibge`:

```sql
JOIN meta.municipio_ibge mi ON mi.codigo_siafi = e.municipio
```

Juntar direto com uma tabela do IBGE não dá erro, dá zero linhas. O de-para
vem do Tesouro Transparente (`fonteca ibge`), nunca do `.rda` do pacote
`qsacnpj`, que é GPL-3.

## Tabelas auxiliares: `cnae` e `natureza`

Carregadas do `Cnaes.zip` e do `Naturezas.zip` do mesmo lote. Todo join com
elas é **LEFT JOIN**: lote antigo ou código sem descrição devolve NULL na
descrição em vez de sumir com a empresa. Motivos, Qualificações e Países
ficam de fora porque nada que a API devolve usa esses códigos.

## A tabela `socio` fica vazia, por decisão

O schema tem `socio`, mas a ingestão **não carrega** o `Socios.zip`:

1. **É dado pessoal.** Abrir mensagem não solicitada com o nome de uma pessoa
   física é o uso mais sensível sob a LGPD, que regula o uso, não a origem.
2. **O sócio raramente atende.** Quem responde o WhatsApp da empresa costuma
   ser recepção ou vendas.
3. **O dado envelhece.** Quadro societário muda, e a base é mensal.

Para análise de grupo econômico, a tabela existe e basta carregar o arquivo.

## Multi-conta e isolamento

Cada chave pertence a uma conta. A chave é guardada como Argon2id, com um
SHA-256 à parte só para localizar a linha; comparação em tempo constante. O
uso por conta (`meta.uso`) tem RLS: cada transação declara a conta com
`SET LOCAL`, e a identidade morre no fim dela. A API roda como `fonteca_api`
(`deploy/papeis.sql`), que não é dono das tabelas: sob o dono a RLS não vale,
e por isso `fonteca serve` se recusa a subir como superusuário, `BYPASSRLS` ou
dono de `meta.uso`.

## Testes sem ingestão

Todo teste usa Postgres real (testcontainers), nunca mock:

```go
pool := bancoDeTeste(t)
schema, _ := store.NomeDoSchema(store.ReferenciaSeed)
store.CriarSchemaDeLote(ctx, pool, schema)
store.SemearLoteDeTeste(ctx, pool, schema)
store.TrocarLoteVigente(ctx, pool, schema, store.ReferenciaSeed, 12)
```

O seed tem 12 estabelecimentos, com um contraexemplo para cada filtro. A
consulta de referência (CNAE 2512800, ativa, com celular, SP, abertura a
partir de 2026-07-01, sem MEI) devolve exatamente **2**. Os testes se ancoram
nesse número.
