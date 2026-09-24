# API

A tela usa a mesma API de qualquer integração: não há rota exclusiva do
frontend. Tudo é JSON; a autenticação é pelo cabeçalho `api-key`.

| Método | Rota | Chave | O que faz |
|---|---|---|---|
| `POST` | `/v1/cnpj/pesquisa` | sim | Lista empresas do filtro, paginada |
| `POST` | `/v1/cnpj/contagem` | sim | Conta empresas do filtro por estado |
| `GET` | `/v1/cnpj/{cnpj}` | sim | Detalhe de um CNPJ (14 dígitos) |
| `GET` | `/v1/uso` | sim | Consumo da conta no mês |
| `GET` | `/v1/health` | não | Estado do serviço e do lote vigente |

Crie chaves com `docker compose run --rm cli conta -nome "Nome"`.

## Pesquisa

```bash
curl -s http://localhost:8080/v1/cnpj/pesquisa \
  -H 'api-key: fnt_live_...' \
  -H 'content-type: application/json' \
  -d '{
    "codigo_atividade_principal": ["5611201", "5611203"],
    "situacao_cadastral": "ATIVA",
    "uf": ["SP", "RJ"],
    "data_abertura": { "ultimos_dias": 90 },
    "mei": { "excluir_optante": true },
    "mais_filtros": { "somente_celular": true },
    "limite": 50
  }'
```

| Campo | Tipo | Obrigatório | Observação |
|---|---|---|---|
| `codigo_atividade_principal` | lista de CNAE (7 dígitos) | sim | |
| `situacao_cadastral` | `ATIVA`, `SUSPENSA`, `INAPTA`, `BAIXADA`, `NULA` | não | padrão `ATIVA` |
| `uf` | lista de siglas | não | vazio = Brasil inteiro |
| `data_abertura.ultimos_dias` | inteiro | não | 0 = sem limite |
| `mei.excluir_optante` | booleano | não | |
| `mais_filtros.somente_celular` | booleano | não | |
| `limite` | 1 a 1000 | não | padrão e teto: 1000 |
| `cursor` | texto | não | o `proximo_cursor` da página anterior |

Resposta:

```json
{
  "resultados": [
    {
      "cnpj": "12345678000190",
      "razao_social": "EXEMPLO COMERCIAL LTDA",
      "nome_fantasia": "EXEMPLO",
      "telefone": "1198888777",
      "whatsapp": "5511998888777",
      "email": "contato@exemplo.com.br",
      "cnae_principal": "5611201",
      "cnae_descricao": "Restaurantes e similares",
      "data_inicio": "2026-09-01T00:00:00Z",
      "uf": "SP",
      "municipio": "SAO PAULO",
      "capital_social": 50000,
      "porte": "01",
      "natureza_juridica": "Sociedade Empresária Limitada"
    }
  ],
  "total": 1,
  "limite_aplicado": 50,
  "lote": "2026-09",
  "proximo_cursor": "MjAy..."
}
```

- **`telefone`** é como a Receita guarda: DDD + 8 dígitos. Para celular, a
  Receita não guarda o nono dígito; **`whatsapp`** já vem com ele restaurado
  (`55` + DDD + `9` + número).
- **Paginação** é por cursor, não por página: repita o pedido com
  `"cursor": "<proximo_cursor>"`. Sem `proximo_cursor`, acabou. Cursor é mais
  rápido que `OFFSET` numa tabela de dezenas de milhões de linhas e não pula
  nem repete linhas.
- **Ordem**: abertura mais recente primeiro.

## Contagem

Mesmo corpo da pesquisa (`limite` e `cursor` são ignorados):

```json
{ "total": 293, "por_uf": { "SP": 110, "PR": 78, "RS": 61, "SC": 44 }, "lote": "2026-09" }
```

A contagem usa exatamente o mesmo filtro da pesquisa: o número por estado é o
que a lista devolveria.

## Detalhe

```bash
curl -s http://localhost:8080/v1/cnpj/12345678000190 -H 'api-key: fnt_live_...'
```

Traz empresa, estabelecimento, situação cadastral e se é optante do MEI.

## Erros

| Código | Quando |
|---|---|
| `400` | Filtro inválido (a mensagem diz qual campo) |
| `401` | Chave ausente ou inválida |
| `404` | CNPJ fora do lote vigente |
| `429` | Limite excedido; o cabeçalho `Retry-After` diz em quantos segundos tentar |
| `503` | Nenhum lote da Receita carregado ainda |
| `500` | Falha interna; a resposta traz só um `correlacao` para procurar no log |

## Limites

Por conta: 60 pedidos por minuto e 10 mil por dia. Por IP, só tentativas sem
chave válida: 10 por minuto. Ajuste no `docker-compose.yml`, nas opções do
`serve` (`-rate-minuto`, `-rate-dia`, `-rate-ip`).
