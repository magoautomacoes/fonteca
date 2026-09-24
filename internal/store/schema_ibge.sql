-- De-para entre o codigo SIAFI (usado pela Receita no arquivo de CNPJ) e o
-- codigo IBGE (usado por IBGE, DATASUS, INEP e demais bases publicas).
-- Permanente: nao muda de mes para mes, entao vive em meta e nao no lote.
-- Fonte primaria: Tesouro Transparente. NUNCA o .rda do qsacnpj, que e GPL-3.
CREATE TABLE IF NOT EXISTS meta.municipio_ibge (
  codigo_siafi integer PRIMARY KEY,
  codigo_ibge  integer NOT NULL,
  nome         text    NOT NULL,
  uf           char(2) NOT NULL
);

CREATE INDEX IF NOT EXISTS municipio_ibge_por_ibge
  ON meta.municipio_ibge (codigo_ibge);

CREATE INDEX IF NOT EXISTS municipio_ibge_por_uf
  ON meta.municipio_ibge (uf);
