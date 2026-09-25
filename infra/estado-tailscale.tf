# Estado do Tailscale da instância cloud num disco que sobrevive às trocas.
#
# Sem ele, cada task nova era uma máquina nova para o Tailscale e pedia um
# certificado novo ao Let's Encrypt, que aceita só 5 por semana para o mesmo
# nome: meia dúzia de deploys num dia derrubou o HTTPS por dois dias. Com o
# estado aqui, o certificado (e a identidade na tailnet) passa de uma task
# para a outra. São kilobytes: custa centavos.

resource "aws_efs_file_system" "tailscale" {
  count          = var.tailscale_authkey == "" ? 0 : 1
  creation_token = "ozymandias-tailscale"
  encrypted      = true
  tags           = { Name = "ozymandias-tailscale" }
}

resource "aws_security_group" "efs_tailscale" {
  count       = var.tailscale_authkey == "" ? 0 : 1
  name        = "ozymandias-efs-tailscale"
  description = "NFS so a partir da instancia cloud"
  vpc_id      = data.aws_vpc.padrao.id
  ingress {
    from_port       = 2049
    to_port         = 2049
    protocol        = "tcp"
    security_groups = [aws_security_group.nuvem.id]
  }
}

resource "aws_efs_mount_target" "tailscale" {
  for_each        = nonsensitive(var.tailscale_authkey == "") ? toset([]) : toset(data.aws_subnets.padrao.ids)
  file_system_id  = aws_efs_file_system.tailscale[0].id
  subnet_id       = each.value
  security_groups = [aws_security_group.efs_tailscale[0].id]
}
