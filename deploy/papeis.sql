-- Tres usuarios, menor privilegio (spec de arquitetura, secao 8).
-- Aplicar como superusuario, DEPOIS de MigrarMeta.
--
-- A senha real vem do .env; estas sao trocadas no deploy com ALTER ROLE.
-- Idempotente: pode rodar sempre.
--
-- Nota: usamos EXECUTE format(...) com current_database() em vez da sintaxe
-- de variavel do psql (:"db"). O psql expande :"db" antes de mandar o SQL ao
-- servidor, mas pool.Exec (Go/pgx) manda o texto cru: :"db" viraria erro de
-- sintaxe. format() com current_database() funciona pelos dois caminhos.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fonteca_ingest') THEN
    CREATE ROLE fonteca_ingest LOGIN PASSWORD 'trocar-no-deploy';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fonteca_api') THEN
    CREATE ROLE fonteca_api LOGIN PASSWORD 'trocar-no-deploy';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fonteca_leitura') THEN
    CREATE ROLE fonteca_leitura LOGIN PASSWORD 'trocar-no-deploy';
  END IF;
END
$$;

-- fonteca_ingest: o unico que escreve dados de lote. CREATE no banco e para
-- criar os schemas lote_AAAA_MM; em meta ele escreve so o que a ingestao
-- escreve (lote vigente e tabelas de referencia), e apenas le contas e chaves.
-- O REVOKE antes torna o script idempotente para bancos que receberam a
-- versao antiga, que dava escrita em todo o schema meta.
DO $$
BEGIN
  EXECUTE format('GRANT CREATE, CONNECT ON DATABASE %I TO fonteca_ingest', current_database());
END
$$;
REVOKE CREATE ON SCHEMA meta FROM fonteca_ingest;
REVOKE ALL ON ALL TABLES IN SCHEMA meta FROM fonteca_ingest;
GRANT USAGE ON SCHEMA meta TO fonteca_ingest;
GRANT SELECT ON ALL TABLES IN SCHEMA meta TO fonteca_ingest;
GRANT INSERT, UPDATE, DELETE ON meta.lote, meta.ddd, meta.municipio_ibge TO fonteca_ingest;

-- fonteca_api: le os lotes, escreve so em meta.uso.
-- Sem CREATE no banco: mesmo comprometida, a API nao cria nem apaga schema.
DO $$
BEGIN
  EXECUTE format('REVOKE CREATE ON DATABASE %I FROM fonteca_api', current_database());
  EXECUTE format('GRANT CONNECT ON DATABASE %I TO fonteca_api', current_database());
END
$$;
GRANT USAGE ON SCHEMA meta TO fonteca_api;
GRANT SELECT ON meta.lote, meta.conta, meta.api_key, meta.ddd, meta.municipio_ibge TO fonteca_api;
GRANT SELECT, INSERT, UPDATE ON meta.uso TO fonteca_api;
-- ultimo_uso e escrito na autenticacao.
GRANT UPDATE (ultimo_uso) ON meta.api_key TO fonteca_api;
-- Nenhuma consulta da API deve passar de segundos (teto de 1.000 linhas por
-- resposta); sem prazo, uma consulta lenta segura conexao do pool mesmo
-- depois que o cliente desistiu. Vale para toda sessao aberta como fonteca_api.
ALTER ROLE fonteca_api SET statement_timeout = '30s';

-- fonteca_leitura: acesso SQL humano, nada em meta.
DO $$
BEGIN
  EXECUTE format('REVOKE CREATE ON DATABASE %I FROM fonteca_leitura', current_database());
  EXECUTE format('GRANT CONNECT ON DATABASE %I TO fonteca_leitura', current_database());
END
$$;

-- Os schemas de lote nascem na ingestao. O DEFAULT PRIVILEGES abaixo cobre
-- SOMENTE as TABELAS que fonteca_ingest criar depois dentro de um schema que
-- ja existe — o Postgres NAO tem equivalente de "default privilege" para
-- USAGE em SCHEMA futuro (default privilege so se aplica a objetos dentro
-- de um schema, nunca ao proprio schema). Ou seja: mesmo com este ALTER, um
-- schema de lote novo nasceria com fonteca_api e fonteca_leitura sem
-- conseguir nem entrar nele (SQLSTATE 42501, "permission denied for
-- schema"), porque o acesso barra na porta antes de chegar as tabelas.
--
-- Por isso o GRANT USAGE ON SCHEMA de cada lote novo, junto com um GRANT
-- SELECT explicito nas tabelas dele (nao so o DEFAULT PRIVILEGES acima, que
-- so valeria se quem criasse o schema fosse sempre exatamente fonteca_ingest),
-- e emitido em codigo Go: internal/store/lote.go, funcao
-- concederUsoDoSchema, chamada dentro de CriarSchemaDeLote — e o unico lugar
-- que sabe o nome do schema no momento em que ele e criado. Este arquivo so
-- cobre o que ja pode ser resolvido antecipadamente (tabelas futuras dentro
-- de schema ja existente) e os schemas de lote que ja existirem no momento
-- em que este script for aplicado (loop abaixo) — sem o loop, rodar este
-- script num banco que ja tem lote carregado nao resolveria o acesso ao
-- lote atual, so aos futuros.
ALTER DEFAULT PRIVILEGES FOR ROLE fonteca_ingest
  GRANT SELECT ON TABLES TO fonteca_api, fonteca_leitura;

-- Cobre os schemas de lote que ja existem no momento do deploy (por exemplo,
-- ao aplicar este script pela primeira vez num banco que ja tem ingestao
-- rodada). Schemas futuros continuam por conta de CriarSchemaDeLote.
DO $$
DECLARE
  s text;
BEGIN
  FOR s IN
    SELECT schema_name FROM information_schema.schemata
    WHERE schema_name ~ '^lote_\d{4}_\d{2}$'
  LOOP
    EXECUTE format('GRANT USAGE ON SCHEMA %I TO fonteca_api, fonteca_leitura', s);
    EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO fonteca_api, fonteca_leitura', s);
  END LOOP;
END
$$;
