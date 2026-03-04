param(
  [string]$ConfigPath = "./smtp-test.local.json"
)

if (-not (Test-Path $ConfigPath)) {
  Write-Error "Config file not found: $ConfigPath"
  exit 1
}

$config = Get-Content $ConfigPath -Raw | ConvertFrom-Json

try {
  $mail = New-Object System.Net.Mail.MailMessage
  $mail.From = $config.from
  $mail.To.Add($config.to)
  $mail.Subject = $config.subject
  $mail.Body = $config.body

  $client = New-Object System.Net.Mail.SmtpClient($config.smtp_host, [int]$config.smtp_port)
  $client.EnableSsl = [bool]$config.use_ssl
  $client.Credentials = New-Object System.Net.NetworkCredential($config.smtp_username, $config.smtp_password)
  $client.DeliveryMethod = [System.Net.Mail.SmtpDeliveryMethod]::Network

  $client.Send($mail)
  Write-Host "SMTP test sent successfully to $($config.to) via $($config.smtp_host):$($config.smtp_port)"
  exit 0
}
catch {
  Write-Error "SMTP test failed: $($_.Exception.Message)"
  exit 1
}
finally {
  if ($mail) { $mail.Dispose() }
  if ($client) { $client.Dispose() }
}