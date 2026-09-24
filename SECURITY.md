# Segurança

Encontrou uma vulnerabilidade? **Não abra issue pública.** Use o relato
privado de vulnerabilidades do GitHub (aba *Security* → *Report a
vulnerability*) com os passos para reproduzir.

O que já está no desenho, para referência de quem audita:

- Chaves de API guardadas como Argon2id, comparadas em tempo constante; a
  chave em claro aparece uma única vez, na criação.
- Uso por conta isolado com RLS no Postgres; a API roda com um papel sem
  posse das tabelas e se recusa a subir como superusuário ou dono.
- Limite por conta e, antes da autenticação, por IP; `X-Forwarded-For` só é
  lido do proxy configurado.
- Erro interno nunca devolve mensagem do banco, só um ID de correlação.
- O banco não é exposto pelo Docker Compose; o Caddy aplica CSP e limita o
  corpo das requisições.

Problemas de uso indevido dos dados (spam, por exemplo) são responsabilidade
de quem opera cada instalação; veja [docs/uso-responsavel.md](docs/uso-responsavel.md).
