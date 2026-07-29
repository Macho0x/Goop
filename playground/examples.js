// Embedded Goop example snippets for the playground.
window.GOOP_EXAMPLES = [
  {
    id: "hello",
    title: "Hello",
    source: `module main

let main () =
  println "Hello, Goop!"
`,
  },
  {
    id: "shapes",
    title: "ADTs & match",
    source: `module main

type Shape =
  | Circle of { radius: float }
  | Rect of { width: float; height: float }
  | Point

let area (s: Shape) : float =
  match s with
  | Circle { radius } -> 3.14159 *. radius *. radius
  | Rect { width; height } -> width *. height
  | Point -> 0.0

let describe (s: Shape) : string =
  match s with
  | Circle { radius } when radius > 10.0 -> "big circle"
  | Circle _ -> "small circle"
  | Rect { width; height } when width = height -> "square"
  | Rect _ -> "rectangle"
  | Point -> "point"

let main () =
  println (describe (Circle { radius = 5.0 }))
`,
  },
  {
    id: "result",
    title: "Result errors",
    source: `module main

type User = { id: int; name: string }
type UserError = NotFound | InvalidInput of string

let findUser (id: int) : (User, UserError) result =
  if id < 0 then Error (InvalidInput "negative id")
  else if id = 42 then Ok { id = 42; name = "Alice" }
  else Error NotFound

let getUserName (id: int) : string =
  match findUser id with
  | Ok u -> u.name
  | Error NotFound -> "unknown"
  | Error (InvalidInput msg) -> "invalid: " ^ msg

let main () =
  println (getUserName 42)
`,
  },
  {
    id: "transparent_go",
    title: "Transparent Go (match)",
    source: `module main

(* Press Compile to see readable Go lowering. *)
let classify (opt: int option) : string =
  match opt with
  | Some x when x > 0 -> "positive"
  | Some x when x < 0 -> "negative"
  | Some _ -> "zero"
  | None -> "none"

let main () =
  println (classify (Some 3))
`,
  },
  {
    id: "maps",
    title: "Maps",
    source: `module main

let main () =
  let table : map[string] int = Map.make () in
  let _ = Map.add table "foo" 1 in
  match Map.get table "foo" with
  | Some n -> println (int_to_string n)
  | None -> println "missing"
`,
  },
  {
    id: "http_hello",
    title: "HTTP (typed import)",
    source: `module main

import go "net/http" {
  val CanonicalHeaderKey : string -> string
}

let main () =
  println (http.CanonicalHeaderKey "content-type")
`,
  },
  {
    id: "branded_ids",
    title: "Branded IDs",
    source: `module Trading.BrandedIds

type order_id = | Order_id of string
type symbol = | Symbol of string

let place (sym: symbol) (oid: order_id) : string =
  "order on branded ids"

let main () : unit =
  let oid = Order_id "ord-1" in
  let sym = Symbol "ETH-USD" in
  println (place sym oid)
`,
  },
  {
    id: "orderbook",
    title: "Order book",
    source: `module Trading.OrderBook

type order_id = | Order_id of string
type symbol = | Symbol of string

type Side = Buy | Sell

type Order =
  { id: order_id
  ; symbol: symbol
  ; side: Side
  ; price: float where price > 0.0
  ; qty: int where qty > 0
  }

type Fill =
  { buyOrder: order_id
  ; sellOrder: order_id
  ; price: float
  ; qty: int where qty > 0
  }

type Book =
  { bids: Order list
  ; asks: Order list
  }

let emptyBook : Book = { bids = []; asks = [] }

let isBetter (side: Side) (a: float) (b: float) : bool =
  match side with
  | Buy -> a > b
  | Sell -> a < b

let rec insertBy (greater: float -> float -> bool) (o: Order) (orders: Order list) : Order list =
  match orders with
  | [] -> [o]
  | first :: rest ->
      if greater o.price first.price then o :: orders
      else first :: insertBy greater o rest

let addOrder (bk: Book) (o: Order) : Book =
  match o.side with
  | Buy -> { bids = insertBy (isBetter Buy) o bk.bids; asks = bk.asks }
  | Sell -> { bids = bk.bids; asks = insertBy (isBetter Sell) o bk.asks }

let bestBid (bk: Book) : float option =
  match bk.bids with
  | [] -> None
  | b :: _ -> Some b.price
`,
  },
  {
    id: "trading_order",
    title: "Order ack",
    source: `module Trading.Order

type OrderAck =
  | Filled of { order_id: string; qty: int }
  | Rejected of { reason: string }
  | PartialFill of { order_id: string; filled: int; remaining: int }

let handleAck (ack: OrderAck) : string =
  match ack with
  | OrderAck.Filled { order_id; qty } -> "filled " ^ order_id
  | OrderAck.Rejected { reason } -> "rejected: " ^ reason
  | OrderAck.PartialFill { order_id; filled; remaining } ->
      "partial " ^ order_id

let route (ack: OrderAck) : string =
  handleAck ack
`,
  },
  {
    id: "arrays",
    title: "Arrays",
    source: `module main

let fill (n: int) (v: int) : int array =
  begin
    let arr = Array.make n v in
    for i = 0 to n - 1 do
      arr.(i) <- v + i
    done;
    arr
  end

let main () =
  let xs = fill 5 10 in
  begin
    assert (Array.length xs = 5);
    assert (xs.(0) = 10 && xs.(4) = 14);
    println "arrays ok"
  end
`,
  },
  {
    id: "exceptions",
    title: "Exceptions",
    source: `module ExceptionsDemo

exception Boom
exception Fail of string

let catch_boom () : string =
  try
    raise Boom
  with
  | Boom -> "caught boom"
  | _ -> "other"

let catch_payload () : string =
  try
    raise (Fail "bad")
  with
  | Fail s -> s
  | _ -> "other"

let main () : unit =
  let chk1 = assert (catch_boom () = "caught boom") in
  assert (catch_payload () = "bad")
`,
  },
];
