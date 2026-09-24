# Tela da Fonteca

React + Vite + TypeScript. Consome a mesma API REST de qualquer integração;
não há rota exclusiva do frontend.

O fluxo tem três etapas na mesma página: **ramo** (busca nas 1.332 atividades
da CNAE), **atividades** (árvore para ajustar os códigos) e **radar** (mapa do
Brasil com a contagem por estado e os filtros de período, situação, celular e
MEI). A lista aparece embaixo, com exportação para planilha.

## Desenvolvimento

Com a API rodando em `localhost:8080` (veja [docs/desenvolvimento.md](../docs/desenvolvimento.md)):

```bash
npm install
npm run dev                                       # tela em :5173, /v1 repassado para :8080
FONTECA_API=http://localhost:18080 npm run dev    # API em outra porta
```

A tela abre em **Demonstração**: empresas fictícias geradas no navegador, com
a mesma regra de filtro da API, para qualquer atividade. Os telefones usam a
faixa 9000-xxxx, fora da numeração móvel em uso. Em **Dados reais**, a chave de
API é digitada na tela e fica só no `sessionStorage` da aba.

## Testes e build

```bash
npm test         # vitest: CSV, formatos, modo demonstração
npx tsc -b && npx oxlint src
npm run build    # gera dist/
```

Em produção, o `Dockerfile` da raiz (alvo `web`) empacota o `dist/` num Caddy
que também faz o proxy de `/v1` e o HTTPS.

## Dados

`src/dados/cnae.json` e `src/dados/mapa-uf.json` são gerados a partir do IBGE
por `scripts/gerar-catalogo-cnae.py` e `scripts/gerar-mapa-uf.py`. O catálogo
é carregado sob demanda, só quando a etapa de ramo abre.
