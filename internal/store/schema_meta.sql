-- Schema permanente do Fonteca. Nunca e dropado pela ingestao.
CREATE SCHEMA IF NOT EXISTS meta;

CREATE TABLE IF NOT EXISTS meta.lote (
  schema       text        PRIMARY KEY,
  referencia   text        NOT NULL,
  vigente      boolean     NOT NULL DEFAULT false,
  linhas       bigint      NOT NULL DEFAULT 0,
  importado_em timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT lote_schema_valido CHECK (schema ~ '^lote_\d{4}_\d{2}$')
);

-- Garante que apenas um lote esteja vigente por vez.
CREATE UNIQUE INDEX IF NOT EXISTS lote_um_vigente
  ON meta.lote (vigente) WHERE vigente;

CREATE TABLE IF NOT EXISTS meta.conta (
  id        uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  nome      text        NOT NULL,
  plano     text        NOT NULL DEFAULT 'interno',
  criada_em timestamptz NOT NULL DEFAULT now(),
  ativa     boolean     NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS meta.api_key (
  hash       text        PRIMARY KEY,
  conta_id   uuid        NOT NULL REFERENCES meta.conta(id) ON DELETE CASCADE,
  nome       text        NOT NULL,
  ultimo_uso timestamptz,
  revogada   boolean     NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS api_key_por_conta ON meta.api_key (conta_id);

-- Coluna de lookup deterministica. O hash Argon2id embute salt aleatorio, entao
-- o mesmo segredo gera hashes diferentes e `WHERE hash = $1` nunca casaria.
-- Este SHA-256 localiza a linha; o Argon2id em `hash` e quem autentica.
--
-- ALTER separado do CREATE TABLE de proposito: a tabela usa IF NOT EXISTS, e
-- uma coluna nova ali dentro seria ignorada em silencio num banco ja existente.
ALTER TABLE meta.api_key ADD COLUMN IF NOT EXISTS hash_lookup text;

CREATE UNIQUE INDEX IF NOT EXISTS api_key_lookup
  ON meta.api_key (hash_lookup) WHERE hash_lookup IS NOT NULL;

CREATE TABLE IF NOT EXISTS meta.uso (
  conta_id          uuid   NOT NULL REFERENCES meta.conta(id) ON DELETE CASCADE,
  dia               date   NOT NULL,
  consultas         bigint NOT NULL DEFAULT 0,
  linhas_retornadas bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (conta_id, dia)
);

-- RLS apenas em meta, onde o isolamento entre contas e real.
-- As tabelas de lote NAO usam RLS: o dado e publico e identico para todos.
ALTER TABLE meta.uso ENABLE ROW LEVEL SECURITY;

-- Sem FOR explicito, o Postgres cria a policy como FOR ALL — e numa policy
-- FOR ALL sem WITH CHECK, a expressao do USING e reaproveitada como WITH CHECK.
-- Ou seja: esta policy protege ESCRITA tambem, nao so leitura. Medido: sob o
-- usuario restrito, INSERT e UPDATE forjando o conta_id de outra conta falham
-- com SQLSTATE 42501. Se algum dia alguem acrescentar FOR SELECT aqui, a
-- protecao de escrita desaparece em silencio.
DROP POLICY IF EXISTS uso_propria_conta ON meta.uso;
CREATE POLICY uso_propria_conta ON meta.uso
  USING (conta_id = current_setting('fonteca.conta_id', true)::uuid);

-- DDDs validos do Brasil (67 codigos). Fica em meta, nao no lote: e referencia
-- estavel e, se a Anatel mudar algo, corrige-se com um UPDATE em vez de
-- reimportar 63 milhoes de linhas.
--
-- Serve para separar telefone plausivel de lixo: a base da Receita tem campos
-- com dado errado (a analise de fase 2 registra UF suja vinda da RFB), entao
-- DDD invalido e esperado. A coluna gerada `celular` valida o NUMERO; esta
-- tabela valida a AREA.
CREATE TABLE IF NOT EXISTS meta.ddd (
  ddd    char(2) PRIMARY KEY,
  uf     char(2) NOT NULL,
  regiao text    NOT NULL
);

INSERT INTO meta.ddd (ddd, uf, regiao) VALUES
  ('11','SP','Sao Paulo'),         ('12','SP','Vale do Paraiba'),
  ('13','SP','Baixada Santista'),  ('14','SP','Bauru'),
  ('15','SP','Sorocaba'),          ('16','SP','Ribeirao Preto'),
  ('17','SP','Sao Jose do Rio Preto'), ('18','SP','Presidente Prudente'),
  ('19','SP','Campinas'),
  ('21','RJ','Rio de Janeiro'),    ('22','RJ','Campos dos Goytacazes'),
  ('24','RJ','Volta Redonda'),
  ('27','ES','Vitoria'),           ('28','ES','Cachoeiro de Itapemirim'),
  ('31','MG','Belo Horizonte'),    ('32','MG','Juiz de Fora'),
  ('33','MG','Governador Valadares'), ('34','MG','Uberlandia'),
  ('35','MG','Pocos de Caldas'),   ('37','MG','Divinopolis'),
  ('38','MG','Montes Claros'),
  ('41','PR','Curitiba'),          ('42','PR','Ponta Grossa'),
  ('43','PR','Londrina'),          ('44','PR','Maringa'),
  ('45','PR','Foz do Iguacu'),     ('46','PR','Francisco Beltrao'),
  ('47','SC','Joinville'),         ('48','SC','Florianopolis'),
  ('49','SC','Chapeco'),
  ('51','RS','Porto Alegre'),      ('53','RS','Pelotas'),
  ('54','RS','Caxias do Sul'),     ('55','RS','Santa Maria'),
  ('61','DF','Brasilia'),
  ('62','GO','Goiania'),           ('64','GO','Rio Verde'),
  ('63','TO','Palmas'),
  ('65','MT','Cuiaba'),            ('66','MT','Rondonopolis'),
  ('67','MS','Campo Grande'),
  ('68','AC','Rio Branco'),
  ('69','RO','Porto Velho'),
  ('71','BA','Salvador'),          ('73','BA','Ilheus'),
  ('74','BA','Juazeiro'),          ('75','BA','Feira de Santana'),
  ('77','BA','Barreiras'),
  ('79','SE','Aracaju'),
  ('81','PE','Recife'),            ('87','PE','Petrolina'),
  ('82','AL','Maceio'),
  ('83','PB','Joao Pessoa'),
  ('84','RN','Natal'),
  ('85','CE','Fortaleza'),         ('88','CE','Juazeiro do Norte'),
  ('86','PI','Teresina'),          ('89','PI','Picos'),
  ('91','PA','Belem'),             ('93','PA','Santarem'),
  ('94','PA','Maraba'),
  ('92','AM','Manaus'),            ('97','AM','Coari'),
  ('95','RR','Boa Vista'),
  ('96','AP','Macapa'),
  ('98','MA','Sao Luis'),          ('99','MA','Imperatriz')
ON CONFLICT (ddd) DO NOTHING;
