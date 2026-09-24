-- Indice principal: serve a consulta-alvo (CNAE + periodo + situacao + UF).
-- Parcial em situacao=2 descarta ~40% das linhas.
CREATE INDEX ON {{schema}}.estabelecimento
  (cnae_principal, data_inicio DESC, situacao, uf) WHERE situacao = 2;

CREATE INDEX ON {{schema}}.estabelecimento (uf, municipio) WHERE situacao = 2;
CREATE INDEX ON {{schema}}.estabelecimento (cnpj_basico);
CREATE INDEX ON {{schema}}.estabelecimento (data_inicio DESC)
  WHERE situacao = 2 AND celular;
CREATE INDEX ON {{schema}}.socio (cnpj_basico);

ANALYZE {{schema}}.estabelecimento;
ANALYZE {{schema}}.empresa;
ANALYZE {{schema}}.socio;
ANALYZE {{schema}}.simples;
