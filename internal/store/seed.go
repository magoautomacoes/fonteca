package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReferenciaSeed e o lote ficticio usado em testes de ambos os planos.
const ReferenciaSeed = "2026-09"

// Doze estabelecimentos escolhidos para cobrir cada eixo da consulta-alvo,
// com contra-exemplos para cada filtro. A contagem esperada da consulta
// completa (CNAE 2512800 + ativa + celular + SP + jul/2026 + nao-MEI) e 2:
// os CNPJs terminados em 0001 e 0004.
const seedSQL = `
INSERT INTO {{schema}}.empresa
  (cnpj_basico, razao_social, natureza_juridica, capital_social, porte) VALUES
  ('10000001', 'METALURGICA ALFA LTDA',           '2062',  150000.00, '03'),
  ('10000002', 'METALURGICA BETA LTDA',           '2062',   80000.00, '01'),
  ('10000003', 'METALURGICA GAMA LTDA',           '2062',   50000.00, '01'),
  ('10000004', 'CONSTRUÇÕES DELTA LTDA',         '2062',  300000.00, '03'),
  ('10000005', 'METALURGICA EPSILON LTDA',        '2135',   20000.00, '01'),
  ('10000006', 'ESTRUTURAS ZETA LTDA',           '2062',  500000.00, '05'),
  ('10000007', 'MADEIRAS ETA LTDA',              '2062',   90000.00, '03'),
  ('10000008', 'FERRAGENS THETA LTDA',           '2062',   60000.00, '01'),
  ('10000009', 'PADARIA IOTA LTDA',              '2062',   30000.00, '01'),
  ('10000010', 'METALURGICA KAPPA LTDA',          '2062',   70000.00, '01'),
  ('10000011', 'METALURGICA LAMBDA LTDA',         '2135',   40000.00, '01'),
  ('10000012', 'METALURGICA MU LTDA',             '2062',  110000.00, '03')
ON CONFLICT (cnpj_basico) DO NOTHING;

INSERT INTO {{schema}}.estabelecimento
  (cnpj, cnpj_basico, matriz_filial, nome_fantasia, situacao,
   data_situacao, data_inicio, cnae_principal, cnae_secundarios,
   uf, municipio, ddd_1, telefone_1, email) VALUES
  -- Atencao: nem toda linha isola um unico filtro. Onde o comentario lista
  -- mais de um motivo, a linha reprova por todos eles. As contagens do teste
  -- continuam valendo; o que nao vale e assumir causa unica por linha.
  -- ALVO 1: CNAE 2512800, ativa, celular, SP, jul/2026, nao-MEI
  ('10000001000101','10000001',1,'ALFA',        2,'2026-07-10','2026-07-10','2512800','', 'SP',7107,'11','98888777','alfa@ex.com'),
  -- ALVO 2: idem
  ('10000004000104','10000004',1,'DELTA',       2,'2026-08-01','2026-08-01','2512800','', 'SP',7107,'11','97777666','delta@ex.com'),
  -- baixada: reprovada pela situacao
  ('10000002000102','10000002',1,'BETA',        8,'2026-08-15','2026-07-20','2512800','', 'SP',7107,'11','96666555','beta@ex.com'),
  -- telefone fixo: reprovada por celular
  ('10000003000103','10000003',1,'GAMA',        2,'2026-07-05','2026-07-05','2512800','', 'SP',7107,'11','33334444','gama@ex.com'),
  -- MEI: reprovada pelo Simples
  ('10000005000105','10000005',1,'EPSILON',     2,'2026-07-12','2026-07-12','2512800','', 'SP',7107,'11','95555444','epsilon@ex.com'),
  -- aberta antes do periodo: reprovada pela data
  ('10000012000112','10000012',1,'MU',          2,'2025-01-20','2025-01-20','2512800','', 'SP',7107,'11','94444333','mu@ex.com'),
  -- outra UF: reprovada pela UF
  ('10000010000110','10000010',1,'KAPPA',       2,'2026-07-18','2026-07-18','2512800','', 'RJ',6001,'21','93333222','kappa@ex.com'),
  -- outra UF e MEI: reprovada por dois filtros
  ('10000011000111','10000011',1,'LAMBDA',      2,'2026-08-05','2026-08-05','2512800','', 'MG',4123,'31','92222111','lambda@ex.com'),
  -- outros CNAEs do mesmo ramo, para testar filtro por multiplos CNAEs
  ('10000006000106','10000006',1,'ZETA',        2,'2026-07-22','2026-07-22','2511000','', 'SP',7107,'11','91111000','zeta@ex.com'),
  -- outro CNAE do ramo, em outra UF
  ('10000007000107','10000007',1,'ETA',         2,'2026-08-10','2026-08-10','1622602','', 'PR',7535,'41','90000999','eta@ex.com'),
  -- outro CNAE do ramo; sem celular
  ('10000008000108','10000008',1,'THETA',       2,'2026-07-30','2026-07-30','4744001','', 'SP',7107,'11','55556666','theta@ex.com'),
  -- CNAE alheio (e tambem sem celular): reprovada pelo ramo
  ('10000009000109','10000009',1,'IOTA',        2,'2026-07-25','2026-07-25','1091102','', 'SP',7107,'11','44445555','iota@ex.com')
ON CONFLICT (cnpj) DO NOTHING;

INSERT INTO {{schema}}.simples (cnpj_basico, opcao_simples, opcao_mei) VALUES
  ('10000001', true,  false),
  ('10000004', false, false),
  ('10000005', true,  true),   -- MEI
  ('10000011', true,  true)    -- MEI
ON CONFLICT (cnpj_basico) DO NOTHING;

INSERT INTO {{schema}}.socio
  (cnpj_basico, nome, qualificacao, data_entrada, faixa_etaria) VALUES
  ('10000001', 'MARIA DA SILVA',   '49', '2026-07-10', 4),
  ('10000001', 'JOSÉ PEREIRA',     '22', '2026-07-10', 5),
  ('10000004', 'ANA SOUZA',        '49', '2026-08-01', 3);

INSERT INTO {{schema}}.municipio (codigo, nome) VALUES
  (7107, 'SAO PAULO'),
  (6001, 'RIO DE JANEIRO'),
  (4123, 'BELO HORIZONTE'),
  (7535, 'CURITIBA')
ON CONFLICT (codigo) DO NOTHING;

-- 1091102 fica SEM descricao de proposito: prova que o LEFT JOIN nao some
-- com a linha quando a auxiliar nao cobre o codigo.
INSERT INTO {{schema}}.cnae (codigo, descricao) VALUES
  ('2512800', 'Fabricação de esquadrias de metal'),
  ('2511000', 'Fabricação de estruturas metálicas'),
  ('1622602', 'Fabricação de esquadrias de madeira e de peças de madeira para instalações industriais e comerciais'),
  ('4744001', 'Comércio varejista de ferragens e ferramentas')
ON CONFLICT (codigo) DO NOTHING;

INSERT INTO {{schema}}.natureza (codigo, descricao) VALUES
  ('2062', 'Sociedade Empresária Limitada'),
  ('2135', 'Empresário (Individual)')
ON CONFLICT (codigo) DO NOTHING;
`

// SemearLoteDeTeste popula um schema de lote ja criado com dados
// deterministicos. E idempotente. Usado pelos testes de ambos os planos:
// os testes de busca e da API nao precisam esperar uma ingestao real.
func SemearLoteDeTeste(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	if err := validarSchema(schema); err != nil {
		return err
	}
	// socio nao tem chave primaria, entao limpa antes para manter idempotencia.
	if _, err := pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s.socio", schema)); err != nil {
		return fmt.Errorf("limpar socios do seed: %w", err)
	}
	sql := strings.ReplaceAll(seedSQL, "{{schema}}", schema)
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("semear %s: %w", schema, err)
	}
	return nil
}
