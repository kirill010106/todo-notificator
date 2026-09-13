$json = @'
{
  "type": "notification",
  "event": "payment.succeeded",
  "object": {
    "id": "317be821-000f-5001-8000-1aac13eb309d",
    "status": "succeeded",
    "amount": {
      "value": "49.00",
      "currency": "RUB"
    },
    "description": "Premium subscription",
    "captured_at": "2026-04-23T14:00:00.000Z",
    "created_at": "2026-04-23T14:00:00.000Z",
    "test": true,
    "paid": true,
    "refundable": true
  }
}
'@

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Invoke-RestMethod -Uri "http://localhost:8082/api/v1/webhooks/yookassa" -Method Post -Body ([System.Text.Encoding]::UTF8.GetBytes($json)) -ContentType "application/json; charset=utf-8"
