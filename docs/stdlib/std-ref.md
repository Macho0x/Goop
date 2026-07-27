# std.ref

**Source:** `std/ref/ref.goop`  
**Import (optional):** `import goop "std.ref"` or `import goop . "std.ref"`

Thin re-export of the prelude `ref` constructor. Dereference (`!r`) and assignment (`r := e`) remain language syntax.

## Exports

| Name | Type | Prelude |
|---|---|---|
| `make` | `'a -> 'a ref` | `ref` |

## Example

```goop
import goop . "std.ref"

let bump () : int =
  let r = make 0 in
  begin
    r := !r + 1;
    !r
  end
```
