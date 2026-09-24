"""生成器测试；从仓库根执行 python -m unittest discover -s test -p 'test_*.py'。"""

import copy
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "tools/protocolgen/generate.py"
SPEC = importlib.util.spec_from_file_location("protocolgen", SCRIPT)
gen = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gen)


def sample():
    return {
        "schema_version": 1, "protocol_version": 1, "encoding": "json",
        "go_package": "protocol", "csharp_namespace": "Test.Protocol",
        "messages": [{"name": "Echo", "id": 0,
                      "request": [{"name": "text", "type": "string"}],
                      "response": [{"name": "text", "type": "string"}]}],
    }


class ProtocolSchemaTests(unittest.TestCase):
    def test_valid_id_boundaries_and_empty_messages(self):
        schema = sample()
        schema["messages"].append({"name": "Max", "id": 0xFFFFFFFF, "request": [], "response": []})
        parsed = gen.validate_schema(schema)
        self.assertEqual([m["id"] for m in parsed["messages"]], [0, 0xFFFFFFFF])
        self.assertIn("type MaxRequest struct{}", gen.render(parsed)["protocol.gen.go"])

    def test_invalid_schema_metadata(self):
        changes = [("schema_version", 2), ("schema_version", True), ("protocol_version", 0),
                   ("protocol_version", 2**32), ("protocol_version", 1.0), ("encoding", "protobuf"),
                   ("go_package", "../escape"), ("go_package", "package"), ("go_package", "main"),
                   ("csharp_namespace", "namespace.Test"), ("csharp_namespace", "Ginx;Bad"),
                   ("messages", []), ("messages", {}), ("typo", 1)]
        for key, value in changes:
            with self.subTest(key=key, value=value), self.assertRaises(gen.SchemaError):
                schema = sample()
                schema[key] = value
                gen.validate_schema(schema)
        for value in ([], None, {"schema_version": 1}):
            with self.subTest(value=value), self.assertRaises(gen.SchemaError):
                gen.validate_schema(value)

    def test_invalid_message_ids_and_names(self):
        for value in (-1, 2**32, True, "1", 1.5, None):
            with self.subTest(value=value), self.assertRaises(gen.SchemaError):
                schema = sample()
                schema["messages"][0]["id"] = value
                gen.validate_schema(schema)
        for value in ("echo", "Echo-Request", "X\npackage main", "", 7):
            with self.subTest(value=value), self.assertRaises(gen.SchemaError):
                schema = sample()
                schema["messages"][0]["name"] = value
                gen.validate_schema(schema)

    def test_duplicate_ids_names_and_generated_symbols(self):
        for name, msg_id in (("Other", 0), ("Echo", 1), ("MsgEcho", 1)):
            with self.subTest(name=name), self.assertRaises(gen.SchemaError):
                schema = sample()
                schema["messages"].append({"name": name, "id": msg_id, "request": [], "response": []})
                if name == "MsgEcho":
                    schema["messages"].append({"name": "EchoRequest", "id": 2, "request": [], "response": []})
                gen.validate_schema(schema)

    def test_bad_fields(self):
        bad = [None, {}, [{"name": "Bad", "type": "string"}], [{"name": "x", "type": "int"}],
               [{"name": "x", "type": "[][]string"}], [{"name": "x", "type": []}],
               [{"name": "x", "type": "string", "optional": 1}],
               [{"name": "x", "type": "string", "optional": None}],
               [{"name": "x", "type": "string", "required": True}],
               [{"name": "x", "type": "string"}] * 2,
               [{"name": "ip", "type": "string"}, {"name": "i_p", "type": "string"}],
               [{"name": "x", "type": "string"}] * 129]
        for value in bad:
            with self.subTest(value=value), self.assertRaises(gen.SchemaError):
                schema = sample()
                schema["messages"][0]["request"] = value
                gen.validate_schema(schema)

    def test_rejects_unknown_or_missing_message_keys(self):
        schema = sample()
        schema["messages"][0]["route"] = "anything"
        with self.assertRaises(gen.SchemaError):
            gen.validate_schema(schema)
        del schema["messages"][0]["route"]
        del schema["messages"][0]["response"]
        with self.assertRaises(gen.SchemaError):
            gen.validate_schema(schema)

    def test_json_duplicates_constants_encoding_and_limits(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "bad.json"
            for data in (b'{"schema_version":1,"schema_version":2}', b'{"a":NaN}',
                         b'{"a":Infinity}', b'{"a":-Infinity}', b'\xff', b'{bad',
                         b' ' * (gen.MAX_SCHEMA_BYTES + 1)):
                with self.subTest(data=data[:50]), self.assertRaises((ValueError, gen.SchemaError)):
                    path.write_bytes(data)
                    gen.load_schema(path)
            path.write_text(json.dumps(sample()), encoding="utf-8-sig")
            self.assertEqual(gen.load_schema(path)["protocol_version"], 1)

    def test_all_types_optional_and_json_tags(self):
        schema = sample()
        fields = [{"name": "field_" + chr(97 + i), "type": kind, "optional": True}
                  for i, kind in enumerate(sorted(gen.TYPES))]
        fields += [{"name": "list_" + chr(97 + i), "type": "[]" + kind} for i, kind in enumerate(sorted(gen.TYPES))]
        schema["messages"][0]["request"] = fields
        go = gen.render(gen.validate_schema(schema))["protocol.gen.go"]
        for f in fields:
            kind = ("*" if f.get("optional") else "") + gen.go_type(f["type"])
            self.assertRegex(go, re.escape(gen.go_field(f["name"])) + r"\s+" + re.escape(kind) + r"\s+`json:")
        self.assertIn('import "encoding/json"', go)
        simple = gen.render(gen.validate_schema(sample()))["protocol.gen.go"]
        self.assertNotIn("import", simple)

    def test_deterministic_output_for_reordered_input(self):
        schema = sample()
        schema["messages"].append({"name": "Other", "id": 2, "request": [], "response": []})
        schema["messages"][0]["request"].append({"name": "count", "type": "uint32"})
        expected = gen.render(gen.validate_schema(schema))
        reversed_schema = dict(reversed(list(schema.items())))
        reversed_schema["messages"] = list(reversed(schema["messages"]))
        reversed_schema["messages"][-1]["request"].reverse()
        self.assertEqual(expected, gen.render(gen.validate_schema(reversed_schema)))

    def test_descriptions_are_not_executable_content(self):
        schema = sample()
        schema["messages"][0]["description"] = '<script>alert(1)</script> | `x` 中文'
        files = gen.render(gen.validate_schema(schema))
        self.assertNotIn("<script>", files["protocol.md"])
        self.assertIn("\\|", files["protocol.md"])
        self.assertNotIn("alert", files["protocol.gen.go"])
        for text in ("bad\nline", "bad\tline", "bad\ud800", "bad\x00", "x" * 1001):
            with self.subTest(text=repr(text)), self.assertRaises(gen.SchemaError):
                schema["messages"][0]["description"] = text
                gen.validate_schema(schema)

    def test_cross_language_ids_match_schema(self):
        schema = gen.load_schema(ROOT / "schema/gameapp.json")
        files = gen.render(schema)
        namespace = {}
        exec(compile(files["protocol_gen.py"], "protocol_gen.py", "exec"), namespace)
        for m in schema["messages"]:
            self.assertEqual(namespace["Msg" + m["name"]], m["id"])
            self.assertIn(f'const Msg{m["name"]} uint32 = {m["id"]}', files["protocol.gen.go"])
            self.assertIn(f'const uint Msg{m["name"]} = {m["id"]}u;', files["ProtocolIds.g.cs"])

    def test_all_type_fixture_is_valid(self):
        schema = gen.load_schema(ROOT / "test/testdata/protocolgen/types.json")
        types = {f["type"] for f in schema["messages"][0]["request"]}
        self.assertEqual(types, gen.TYPES | {"[]" + t for t in gen.TYPES})
        self.assertIn("4294967295u", gen.render(schema)["ProtocolIds.g.cs"])


class ProtocolCompatibilityTests(unittest.TestCase):
    def test_documentation_changes_do_not_require_version_bump(self):
        old = gen.validate_schema(sample())
        new = copy.deepcopy(old)
        new["messages"][0]["description"] = "new documentation"
        new["messages"][0]["request"][0]["description"] = "new field documentation"
        gen.check_compatibility(new, old)

    def test_additions_require_version_bump(self):
        old = gen.validate_schema(sample())
        new = copy.deepcopy(old)
        new["messages"].append({"name": "New", "id": 2, "request": [], "response": []})
        with self.assertRaisesRegex(gen.SchemaError, "increase"):
            gen.check_compatibility(gen.validate_schema(new), old)
        new["protocol_version"] = 2
        gen.check_compatibility(gen.validate_schema(new), old)

    def test_breaking_changes_rejected_even_with_version_bump(self):
        old = gen.validate_schema(sample())
        for action in ("id", "name", "type", "remove_field", "add_optional", "optional", "package", "namespace", "downgrade"):
            with self.subTest(action=action), self.assertRaises(gen.SchemaError):
                new = copy.deepcopy(old)
                new["protocol_version"] = 2
                msg = new["messages"][0]
                if action == "id":
                    msg["id"] = 2
                elif action == "name":
                    msg["name"] = "Renamed"
                elif action == "type":
                    msg["request"][0]["type"] = "uint32"
                elif action == "remove_field":
                    msg["response"] = []
                elif action == "add_optional":
                    msg["request"].append({"name": "extra", "type": "bool", "optional": True})
                elif action == "optional":
                    msg["response"][0]["optional"] = True
                elif action == "package":
                    new["go_package"] = "other"
                elif action == "namespace":
                    new["csharp_namespace"] = "Other"
                else:
                    old = copy.deepcopy(old)
                    old["protocol_version"] = 3
                gen.check_compatibility(gen.validate_schema(new), old)


class ProtocolCommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ginx protocol ")
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.schema = self.directory / "input.json"
        self.schema.write_text(json.dumps(sample()), encoding="utf-8")
        self.out = self.directory / "output with spaces"

    def run_cli(self, *args, code=0):
        result = subprocess.run([sys.executable, str(SCRIPT), *map(str, args)], cwd=self.directory,
                                capture_output=True, text=True, encoding="utf-8", timeout=15,
                                env={**os.environ, "PYTHONUTF8": "1", "PYTHONDONTWRITEBYTECODE": "1"})
        self.assertEqual(result.returncode, code, result.stdout + result.stderr)
        self.assertNotIn("Traceback", result.stderr)
        return result

    def custom(self, *args, code=0):
        return self.run_cli("--schema", self.schema, "--out", self.out, *args, code=code)

    def test_generation_check_and_noop_do_not_rewrite(self):
        self.custom()
        before = {p.name: (p.read_bytes(), p.stat().st_mtime_ns) for p in self.out.iterdir()}
        self.assertEqual(len(before), 5)
        self.custom("--check")
        self.custom()
        self.assertEqual(before, {p.name: (p.read_bytes(), p.stat().st_mtime_ns) for p in self.out.iterdir()})

    def test_check_missing_or_outdated_is_read_only(self):
        self.custom("--check", code=1)
        self.assertFalse(self.out.exists())
        self.custom()
        changed = self.out / "protocol_gen.py"
        changed.write_text(changed.read_text() + "# drift\n", encoding="utf-8")
        before = changed.read_bytes()
        self.custom("--check", code=1)
        self.assertEqual(changed.read_bytes(), before)
        self.custom()
        self.custom("--check")

    def test_check_ignores_crlf_only_differences(self):
        self.custom()
        for path in self.out.iterdir():
            path.write_bytes(path.read_bytes().replace(b"\n", b"\r\n"))
        self.custom("--check")

    def test_custom_schema_requires_explicit_output(self):
        self.run_cli("--schema", self.schema, code=2)
        self.run_cli("--schema", self.schema, "--validate")
        self.assertFalse(self.out.exists())

    def test_validate_does_not_generate(self):
        self.custom("--validate")
        self.assertFalse(self.out.exists())
        self.schema.write_text("{bad", encoding="utf-8")
        self.custom(code=2)
        self.assertFalse(self.out.exists())

    def test_refuses_overwriting_user_file_before_any_write(self):
        self.out.mkdir()
        user_file = self.out / "ProtocolIds.g.cs"
        user_file.write_text("// hand-written code\n", encoding="utf-8")
        self.custom(code=2)
        self.assertEqual(list(self.out.iterdir()), [user_file])
        self.assertEqual(user_file.read_text(), "// hand-written code\n")

    def test_preserves_other_files(self):
        self.out.mkdir()
        user_file = self.out / "notes.txt"
        user_file.write_text("keep", encoding="utf-8")
        self.custom()
        self.assertEqual(user_file.read_text(), "keep")

    def test_baseline_schema_and_lock(self):
        self.custom()
        baseline = self.directory / "baseline.lock.json"
        baseline.write_bytes((self.out / "protocol.lock.json").read_bytes())
        self.custom("--check", "--baseline", baseline)
        self.custom("--check", "--baseline", self.out / "protocol.lock.json")
        self.custom("--validate", "--baseline", self.schema)
        self.custom("--baseline", self.out / "protocol.lock.json", code=2)
        current = sample()
        current["messages"][0]["id"] = 3
        current["protocol_version"] = 2
        self.schema.write_text(json.dumps(current), encoding="utf-8")
        self.custom("--baseline", baseline, code=2)

    def test_default_paths_are_independent_of_cwd(self):
        self.run_cli("--check")

    def test_rejects_input_as_generated_target(self):
        self.out.mkdir()
        self.schema = self.out / "protocol.lock.json"
        self.schema.write_text(json.dumps(sample()), encoding="utf-8")
        self.custom(code=2)
        self.assertEqual(len(list(self.out.iterdir())), 1)

    def test_rejects_symlink_outputs(self):
        self.out.mkdir()
        link = self.out / "protocol.gen.go"
        try:
            link.symlink_to(self.schema)
        except (OSError, NotImplementedError):
            self.skipTest("symlink creation not available")
        before = self.schema.read_bytes()
        self.custom(code=2)
        self.assertEqual(self.schema.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
