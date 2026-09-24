CREATE SCHEMA IF NOT EXISTS {{schema}};

CREATE TABLE {{schema}}.estabelecimento (
  cnpj             char(14) PRIMARY KEY,
  cnpj_basico      char(8)  NOT NULL,
  matriz_filial    smallint,
  nome_fantasia    text,
  situacao         smallint,
  data_situacao    date,
  data_inicio      date,
  cnae_principal   char(7),
  cnae_secundarios text,
  uf               char(2),
  municipio        integer,
  ddd_1            varchar(4),
  telefone_1       varchar(9),
  ddd_2            varchar(4),
  telefone_2       varchar(9),
  email            text,
  -- ATENCAO antes de "corrigir" esta regra. A Anatel definiu em 2016 que
  -- celular tem 9 digitos comecando em 9 — mas a RECEITA NAO GUARDA O NONO
  -- DIGITO. Medido no lote 2026-09: dos 1.240.332 telefones preenchidos,
  -- TODOS tem 8 digitos e NENHUM tem 9. Exigir 9 digitos zera a base inteira.
  --
  -- Nesta base, portanto, "8 digitos comecando em 9" E um celular, com o nono
  -- digito truncado na origem. Quem for discar acrescenta o 9 na frente:
  -- 98960051 vira 998960051.
  --
  -- Validacao de DDD nao entra aqui de proposito: a coluna e STORED, entao
  -- mudar a regra exige reimportar o lote. O que e estavel mora na coluna; o
  -- que e volatil (faixa de DDD) fica em meta.ddd, corrigivel sem recarga.
  celular          boolean GENERATED ALWAYS AS
                   (length(telefone_1) = 8 AND left(telefone_1, 1) = '9') STORED
);

CREATE TABLE {{schema}}.empresa (
  cnpj_basico       char(8) PRIMARY KEY,
  razao_social      text,
  natureza_juridica char(4),
  capital_social    numeric(18,2),
  porte             char(2)
);

CREATE TABLE {{schema}}.socio (
  cnpj_basico  char(8) NOT NULL,
  nome         text,
  qualificacao char(2),
  data_entrada date,
  faixa_etaria smallint
);

CREATE TABLE {{schema}}.simples (
  cnpj_basico   char(8) PRIMARY KEY,
  opcao_simples boolean NOT NULL DEFAULT false,
  opcao_mei     boolean NOT NULL DEFAULT false
);

CREATE TABLE {{schema}}.municipio (
  codigo integer PRIMARY KEY,
  nome   text NOT NULL
);

-- Auxiliares: traduzem codigo em texto. Vem com o lote mensal (a Receita
-- revisa descricoes), por isso moram no schema de lote e nao em meta.
CREATE TABLE {{schema}}.cnae (
  codigo    char(7) PRIMARY KEY,
  descricao text NOT NULL
);

CREATE TABLE {{schema}}.natureza (
  codigo    char(4) PRIMARY KEY,
  descricao text NOT NULL
);
