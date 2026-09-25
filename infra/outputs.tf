output "bucket" {
  value = aws_s3_bucket.midia.bucket
}

# Cole isto em ~/.aws/config. O aws_signing_helper lê o certificado e a chave
# do Keychain e entrega credenciais de 1 h ao SDK do Ozymandias.
output "perfil_aws" {
  value = <<-EOT
    [profile ozymandias]
    region = ${var.regiao}
    credential_process = aws_signing_helper credential-process --cert-selector "Key=x509Subject,Value=CN=${var.cn_do_mac}" --trust-anchor-arn ${aws_rolesanywhere_trust_anchor.mac.arn} --profile-arn ${aws_rolesanywhere_profile.mac.arn} --role-arn ${aws_iam_role.mac.arn}
  EOT
}

output "configurar_nas" {
  value = <<-EOT
    nas config set nuvem.bucket ${aws_s3_bucket.midia.bucket}
    nas config set nuvem.regiao ${var.regiao}
    nas config set nuvem.perfil ozymandias
    nas modo hibrido
  EOT
}
