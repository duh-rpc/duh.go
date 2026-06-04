# 2. oneOf permitted only as a nested, key-tagged union

Date: 2026-06-03

## Status

Accepted

## Context

DUH-RPC serves the same OpenAPI schema as both `application/json` and `application/protobuf`.
A schema construct is therefore usable only if a single schema describes it truthfully on both
wires.

OpenAPI `oneOf` admits two serializations:

- **Discriminated (flat, value-tagged).** A top-level `oneOf` of `$ref` variants plus a
  `discriminator` hoists the selected variant's fields to the top level and names the variant by a
  tag value: `{"type": "cat", "pet_name": "Whiskers"}`.
- **Nested (key-tagged).** An object with one optional property per variant; the present key names
  the variant and its payload nests beneath it: `{"cat_event": {"pet_name": "Whiskers"}}`.

proto3's `oneof` serializes to the nested, key-tagged shape and has no inline-flattening form, so
the flat shape is structurally unreachable from a `.proto`. The nested shape is reachable: a proto3
`oneof` and a set of optional message fields both emit it, byte-identically — verified for the JSON
and binary wires with the standard protobuf JSON mapping, with snake_case keys preserved via
`json_name` annotations.

Two earlier positions were inconsistent. Tooling banned all `oneOf` outright — broader than wire
compatibility requires, rejecting a provably compatible pattern. The written specification permitted
only discriminated `oneOf` — the one form that cannot be served as protobuf.

`allOf` and `anyOf` raise a separate problem: protobuf has no merge or "one-or-more" construct, so
they are ambiguous to map regardless of wire shape.

## Decision

We will permit `oneOf` only as a nested, key-tagged union: an object schema with one optional
`$ref` property per variant and a `oneOf` of single-`required` branches and no `discriminator`. This
form maps to a protobuf `oneof`. We will prohibit the discriminated/flat `oneOf`, and we will
continue to prohibit `allOf` and `anyOf`.

## Consequences

- A previously rejected but provably compatible union construct becomes expressible in schemas.
- The code generator must emit a protobuf `oneof` for the permitted form; the linter's
  discriminator-specific checks become dead and are removed.
- The written specification's union section now contradicts this decision until it is updated;
  it currently permits the prohibited discriminated form.
- A protobuf `oneof` rejects multi-variant input on decode, aligning generated code with the
  "exactly one" intent of the constraint. It cannot represent an array-typed variant payload
  (proto3 forbids `repeated` inside a `oneof`), so such schemas are rejected at generation.
- `allOf` and `anyOf` remain unsupported for an unrelated reason, so this decision does not open a
  path to general schema composition.
- The wire-equality guarantee depends on the default protobuf JSON mapping. Enabling non-default
  field naming (lowerCamelCase output) would break it and must be avoided.
