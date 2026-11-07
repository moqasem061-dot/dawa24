# Orgasoft sample payloads (from Orgasoft via Postman)

These are the real samples. They are the source of truth for field spelling. Do not rename these fields.

## Product sync, camelCase

```json
{
  "items": [
    {
      "externalProductCode": "4001",
      "arName": "منتج مياه 1",
      "stock": 1976,
      "publicPrice": 1305.46,
      "discount": 2,
      "tax": 14,
      "isQuota": false,
      "quotaQuantity": 0,
      "barcode": "6221790408214",
      "expiryDate": "2028-01-10"
    }
  ]
}
```

## Product sync, PascalCase (11 Mar 2026)

```json
{
  "items": [
    {
      "ExternalProductCode": "ORGA-100",
      "ArName": "منتج اورجا سوفت",
      "Stock": 50,
      "PublicPrice": 250,
      "Discount": 0,
      "Tax": 14,
      "IsQuota": false,
      "QuotaQuantity": 0,
      "Barcode": "8888888888"
    }
  ]
}
```

## Order data (11 Mar 2026)

The array wrapper is part of the sample. Orgasoft has not explained this payload. The spec assumes it is the import acknowledgement.

```json
[
  {
    "order_number": "{{orderNumber}}",
    "orga_orderID": "ORG-9921",
    "items": [
      {
        "sku": "MED-001",
        "name": "باراسيتامول",
        "qty": 1,
        "price": 15.00,
        "discount": 5,
        "taxes": 1.50
      }
    ]
  }
]
```

## Rules these samples set

- `quotaQuantity` can be 0 with `isQuota: false`. Zero must not fail validation.
- `ExpiryDate` is optional.
- Codes can be numeric text or contain letters, such as `ORGA-100`.
- `publicPrice`, `discount`, and `tax` arrive as JSON numbers.
- `arName` is the only name field. No English name is sent.
- `taxes` in the order sample looks like an amount, not a percentage. This conflicts with `tax: 14` in the product samples. Confirm with Orgasoft.
