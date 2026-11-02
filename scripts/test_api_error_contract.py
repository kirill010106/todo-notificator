"""Consistency checks for the documented error contract, not a full OAS validator."""
import json
from pathlib import Path
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[1]


class ErrorContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.document = yaml.safe_load((ROOT / "docs/openapi.yaml").read_text(encoding="utf-8"))
        cls.schema = cls.document["components"]["schemas"]["ErrorResponse"]

    def test_common_schema_matches_wire_variants(self):
        required = {"code", "message", "error", "request_id", "field_errors"}
        self.assertEqual(set(self.schema["required"]), required)
        props = self.schema["properties"]
        self.assertEqual(props["status"]["oneOf"], [
            {"type": "string", "enum": ["Error"]},
            {"type": "integer", "minimum": 400, "maximum": 599},
        ])
        self.assertTrue(props["field_errors"]["nullable"])
        for name in ("field_errors", "validation_errors"):
            self.assertEqual(props[name]["type"], "object")
            self.assertEqual(props[name]["additionalProperties"], {"type": "string"})
        alias = self.document["components"]["schemas"]["APIError"]
        self.assertEqual(alias["allOf"], [{"$ref": "#/components/schemas/ErrorResponse"}])

        shapes = 0
        directory = ROOT / "backend/internal/http-server/router/testdata"
        for filename in ("error_contract_baseline.json", "error_contract_postgres_baseline.json"):
            fixtures = json.loads((directory / filename).read_text(encoding="utf-8"))
            for name, fixture in fixtures.items():
                if fixture["status"] < 400 or "json" not in fixture:
                    continue
                shapes += 1
                with self.subTest(filename=filename, scenario=name):
                    body = fixture["json"]
                    self.assertTrue(required <= body.keys())
                    for field in required - {"field_errors"}:
                        self.assertIsInstance(body[field], str)
                    self.assertTrue(body["code"] and body["request_id"])
                    self.assertEqual(body["message"], body["error"])
                    if "status" in body:
                        self.assertTrue(body["status"] == "Error" or body["status"] == fixture["status"])
                    for field in ("field_errors", "validation_errors"):
                        if field not in body:
                            continue
                        if body[field] is None:
                            self.assertTrue(props[field].get("nullable", False))
                        else:
                            self.assertIsInstance(body[field], dict)
                            self.assertTrue(all(isinstance(value, str) for value in body[field].values()))
        self.assertEqual(shapes, 52)

    def test_every_operation_documents_rate_and_internal_failures(self):
        operations = 0
        for path, item in self.document["paths"].items():
            for method, operation in item.items():
                if method not in {"get", "post", "put", "patch", "delete", "head", "options"}:
                    continue
                operations += 1
                with self.subTest(path=path, method=method):
                    responses = operation["responses"]
                    self.assertIn("500", responses)
                    if path == "/webhooks/yookassa":
                        limited = responses["429"]
                        self.assertEqual(set(limited["content"]), {"text/plain"})
                    else:
                        self.assertEqual(responses["429"], {"$ref": "#/components/responses/RateLimited"})
                        limited = self.document["components"]["responses"]["RateLimited"]
                        self.assertEqual(set(limited["content"]), {"application/json"})
                    self.assertTrue(limited["headers"]["Retry-After"]["required"])
                    self.assertEqual(limited["headers"]["Retry-After"]["schema"]["type"], "integer")
        self.assertEqual(operations, 48)

    def test_internal_references_resolve(self):
        def visit(value):
            if isinstance(value, list):
                for child in value:
                    visit(child)
            elif isinstance(value, dict):
                if "$ref" in value:
                    reference = value["$ref"]
                    self.assertTrue(reference.startswith("#/"), reference)
                    target = self.document
                    for component in reference[2:].split("/"):
                        target = target[component.replace("~1", "/").replace("~0", "~")]
                for child in value.values():
                    visit(child)
        visit(self.document)


if __name__ == "__main__":
    unittest.main()
