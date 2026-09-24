# Uso responsável

A Fonteca organiza um cadastro que a Receita Federal publica como dado aberto.
Isso não torna qualquer uso permitido. Esta página resume o que vale saber
antes de prospectar. Não é orientação jurídica.

## O dado é público, o uso é regulado

A LGPD regula o **uso** de dados pessoais, inclusive os que foram tornados
públicos (art. 7º, §§ 3º e 4º). No cadastro de CNPJ há dados pessoais,
sobretudo de empresário individual e MEI: o nome da empresa costuma ser o nome
da pessoa, e o celular e o e-mail costumam ser pessoais.

Usar esses dados para prospecção é possível, mas com cuidado:

- **Finalidade compatível.** Abordagem comercial relacionada à atividade da
  empresa ("vi que vocês abriram um restaurante; fornecemos equipamentos para
  cozinha") é bem diferente de usar o contato para qualquer coisa.
- **Mínimo necessário.** Use o que precisa para aquela abordagem.
- **Direito de oposição.** Se alguém pedir para não ser contatado, pare, e
  registre para não voltar a contatar.
- **Transparência.** Diga de onde veio o contato se perguntarem: cadastro
  público de CNPJ da Receita Federal.

A Fonteca, de propósito, não carrega o quadro de sócios: é o dado pessoal mais
sensível e o menos útil para abordar a empresa.

## WhatsApp: nada de disparo em massa

A política do WhatsApp proíbe mensagem em massa e automatizada para quem não
pediu. Números que fazem isso são banidos, às vezes em horas. O botão de
WhatsApp da Fonteca abre **uma** conversa por vez, de propósito.

Para escalar de forma legítima, use a API oficial do WhatsApp Business com
modelos de mensagem aprovados e respeite o opt-in.

## O contato pode não ser da empresa

O telefone e o e-mail são os que a empresa declarou ao abrir o CNPJ, e muitas
vezes quem abre é a contabilidade, que põe o próprio contato no cadastro. Um
e-mail como `contato@contabilidadeexemplo.com.br` provavelmente é do contador.
Aborde com isso em mente.

## Dados podem estar desatualizados

A Receita não confere o telefone nem o e-mail, e o cadastro é atualizado uma
vez por mês. Um número pode estar errado, desativado ou ter mudado de dono.

## Se você expõe a Fonteca na internet

- Mantenha a API atrás de chave (é o padrão) e emita uma chave por pessoa.
- Não publique a base nem exportações com contatos.
- Faça backup do schema `meta` (contas e uso) e proteja o `.env`.

A Fonteca não tem vínculo com a Receita Federal. Quem usa é responsável pelo
próprio uso dos dados.
