-- Consultas uteis da Fonteca, para rodar direto no psql.
-- Troque lote_2026_09 pelo lote vigente: SELECT schema FROM meta.lote WHERE vigente;
SET search_path TO lote_2026_09, public;

-- 1) A CONSULTA-ALVO: empresas recem-abertas de um ramo (troque os CNAEs)
--    Ajuste UFs, dias e CNAEs conforme a campanha.
SELECT e.cnpj,
       m.razao_social,
       e.nome_fantasia,
       e.ddd_1 || e.telefone_1              AS telefone,
       '55' || e.ddd_1 || '9' || e.telefone_1 AS whatsapp,  -- a Receita trunca o 9
       e.email,
       e.cnae_principal,
       e.data_inicio,
       e.uf,
       mu.nome                              AS municipio,
       m.capital_social,
       m.porte
FROM estabelecimento e
JOIN empresa m USING (cnpj_basico)
LEFT JOIN simples s USING (cnpj_basico)
LEFT JOIN municipio mu ON mu.codigo = e.municipio
WHERE e.cnae_principal = ANY(ARRAY['5611201','5611203'])  -- restaurantes e lanchonetes
  AND e.situacao = 2                        -- ATIVA
  AND e.celular                             -- tem celular
  AND e.uf = ANY(ARRAY['SP'])
  AND e.data_inicio >= CURRENT_DATE - 90    -- abertas nos ultimos 90 dias
  AND COALESCE(s.opcao_mei, false) = false  -- excluindo MEI
ORDER BY e.data_inicio DESC;

-- 2) Quantas empresas por CNAE (o universo carregado)
SELECT cnae_principal, count(*) AS total,
       count(*) FILTER (WHERE situacao = 2) AS ativas,
       count(*) FILTER (WHERE situacao = 2 AND celular) AS ativas_com_celular
FROM estabelecimento
GROUP BY cnae_principal ORDER BY total DESC;

-- 3) Distribuicao por UF (so ativas com celular)
SELECT uf, count(*) AS leads_potenciais
FROM estabelecimento
WHERE situacao = 2 AND celular
GROUP BY uf ORDER BY 2 DESC;

-- 4) Empresas abertas por mes (tendencia do mercado)
SELECT date_trunc('month', data_inicio)::date AS mes, count(*)
FROM estabelecimento
WHERE situacao = 2 AND data_inicio >= CURRENT_DATE - 365
GROUP BY 1 ORDER BY 1 DESC;

-- 5) As maiores por capital social
SELECT m.razao_social, m.capital_social, e.uf, mu.nome AS municipio,
       e.ddd_1 || e.telefone_1 AS telefone
FROM estabelecimento e
JOIN empresa m USING (cnpj_basico)
LEFT JOIN municipio mu ON mu.codigo = e.municipio
WHERE e.situacao = 2 AND e.cnae_principal = '2512800'
ORDER BY m.capital_social DESC NULLS LAST
LIMIT 20;

-- 6) Onde estao concentradas (top municipios)
SELECT mu.nome, e.uf, count(*) AS empresas
FROM estabelecimento e
JOIN municipio mu ON mu.codigo = e.municipio
WHERE e.situacao = 2 AND e.celular
GROUP BY 1,2 ORDER BY 3 DESC LIMIT 20;

-- 7) Qual lote esta carregado
SELECT schema, referencia, linhas, importado_em FROM meta.lote WHERE vigente;
