import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ApiError,
  cancelOrder,
  checkout,
  fetchOrder,
  fetchProducts,
  markPaid,
  markShipped,
  startPrint,
  type Order,
  type OrderStatus,
  type Product,
  type Variant,
} from "./api";
import { formatMoney, newIdempotencyKey } from "./money";

type TicketLine = { variant: Variant; productName: string; quantity: number };

const PIPELINE: OrderStatus[] = ["PENDING", "PAID", "PRINTING", "SHIPPED"];

export default function App() {
  const [products, setProducts] = useState<Product[]>([]);
  const [ticket, setTicket] = useState<TicketLine[]>([]);
  const [email, setEmail] = useState("buyer@example.com");
  const [key, setKey] = useState(newIdempotencyKey);
  const [order, setOrder] = useState<Order | null>(null);
  const [lookup, setLookup] = useState("");
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const loadCatalog = useCallback(async () => {
    setProducts(await fetchProducts());
  }, []);

  useEffect(() => {
    loadCatalog().catch((err: unknown) => setNotice(message(err)));
  }, [loadCatalog]);

  const totalCents = useMemo(
    () => ticket.reduce((sum, line) => sum + line.variant.unitPrice.amountCents * line.quantity, 0),
    [ticket],
  );

  function add(productName: string, variant: Variant) {
    setTicket((cur) => {
      const i = cur.findIndex((l) => l.variant.id === variant.id);
      if (i >= 0) {
        const next = [...cur];
        next[i] = { ...next[i], quantity: Math.min(20, next[i].quantity + 1) };
        return next;
      }
      return [...cur, { variant, productName, quantity: 1 }];
    });
  }

  async function place() {
    setBusy(true);
    setNotice(null);
    try {
      const placed = await checkout({
        idempotencyKey: key,
        email,
        lines: ticket.map((l) => ({ variantId: l.variant.id, quantity: l.quantity })),
      });
      setOrder(placed);
      setLookup(placed.id);
      await loadCatalog();
      setNotice(`Order ${placed.id} · ${placed.status}. Submit again with this key to replay.`);
    } catch (err) {
      setNotice(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function runStatus(fn: (id: string) => Promise<Order>) {
    if (!order) return;
    setBusy(true);
    setNotice(null);
    try {
      const next = await fn(order.id);
      setOrder(next);
      await loadCatalog();
    } catch (err) {
      setNotice(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function look() {
    setBusy(true);
    setNotice(null);
    try {
      const found = await fetchOrder(lookup.trim());
      setOrder(found);
      if (!found) setNotice("No order with that id.");
    } catch (err) {
      setNotice(message(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="page">
      <header className="mast">
        <div>
          <p className="eyebrow">Reliable Commerce Core</p>
          <h1>Catalog, checkout, fulfillment.</h1>
        </div>
        <p className="lede">
          Integer cents. One idempotency key per checkout. Inventory is reserved, not hoped.
        </p>
      </header>

      {notice && <p className="notice">{notice}</p>}

      <main className="grid">
        <section>
          <h2>Catalog</h2>
          {products.map((p) => (
            <article key={p.id} className="card">
              <h3>{p.name}</h3>
              <p className="muted">{p.description}</p>
              <ul className="variants">
                {p.variants.map((v) => (
                  <li key={v.id}>
                    <div>
                      <strong>
                        {v.size} · {v.finish}
                      </strong>
                      <span className="mono">{v.sku}</span>
                      <span>{formatMoney(v.unitPrice.amountCents, v.unitPrice.currency)}</span>
                      <span className={v.available < 5 ? "warn" : "muted"}>{v.available} available</span>
                    </div>
                    <button type="button" disabled={v.available < 1} onClick={() => add(p.name, v)}>
                      Add
                    </button>
                  </li>
                ))}
              </ul>
            </article>
          ))}
        </section>

        <section>
          <h2>Checkout</h2>
          <article className="card">
            {ticket.length === 0 ? (
              <p className="muted">Add a variant. This is the whole cart.</p>
            ) : (
              <ul className="ticket">
                {ticket.map((l) => (
                  <li key={l.variant.id}>
                    <span>
                      {l.productName} · {l.variant.size} · {l.variant.finish}
                    </span>
                    <label>
                      qty
                      <input
                        type="number"
                        min={1}
                        max={20}
                        value={l.quantity}
                        onChange={(e) => {
                          const quantity = Number(e.target.value);
                          setTicket((cur) =>
                            cur.map((row) => (row.variant.id === l.variant.id ? { ...row, quantity } : row)),
                          );
                        }}
                      />
                    </label>
                  </li>
                ))}
              </ul>
            )}
            <p className="total">
              Total {formatMoney(totalCents, ticket[0]?.variant.unitPrice.currency ?? "USD")}
            </p>
            <label className="stack">
              Email
              <input value={email} onChange={(e) => setEmail(e.target.value)} />
            </label>
            <label className="stack">
              Idempotency key
              <span className="keyrow">
                <input value={key} onChange={(e) => setKey(e.target.value)} spellCheck={false} />
                <button type="button" onClick={() => setKey(newIdempotencyKey())}>
                  New key
                </button>
              </span>
            </label>
            <button type="button" className="primary" disabled={busy || ticket.length === 0} onClick={() => void place()}>
              Place order
            </button>
            <p className="hint">
              Place twice with the same key and payload: one order. Change the email, keep the key: conflict.
            </p>
          </article>
        </section>

        <section>
          <h2>Fulfillment</h2>
          <article className="card">
            <label className="stack">
              Order id
              <span className="keyrow">
                <input value={lookup} onChange={(e) => setLookup(e.target.value)} spellCheck={false} />
                <button type="button" disabled={busy || !lookup.trim()} onClick={() => void look()}>
                  Load
                </button>
              </span>
            </label>
            {order && (
              <>
                <ol className="pipe">
                  {PIPELINE.map((s) => (
                    <li key={s} className={pipeClass(order.status, s)}>
                      {s}
                    </li>
                  ))}
                </ol>
                <p>
                  <span className="mono">{order.id}</span>
                  <br />
                  {order.email} · {formatMoney(order.total.amountCents, order.total.currency)}
                </p>
                <div className="actions">
                  <button type="button" disabled={busy} onClick={() => void runStatus(markPaid)}>
                    Mark paid
                  </button>
                  <button type="button" disabled={busy} onClick={() => void runStatus(startPrint)}>
                    Start print
                  </button>
                  <button type="button" disabled={busy} onClick={() => void runStatus(markShipped)}>
                    Ship
                  </button>
                  <button type="button" disabled={busy} onClick={() => void runStatus(cancelOrder)}>
                    Cancel
                  </button>
                </div>
                <p className="hint">Cancel releases the reservation. Ship converts it into on-hand.</p>
              </>
            )}
          </article>
        </section>
      </main>
    </div>
  );
}

function pipeClass(current: OrderStatus, step: OrderStatus): string {
  if (current === "CANCELLED") return "cancelled";
  const ci = PIPELINE.indexOf(current);
  const si = PIPELINE.indexOf(step);
  if (si < ci) return "done";
  if (si === ci) return "now";
  return "";
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    return err.code ? `${err.code}: ${err.message}` : err.message;
  }
  if (err instanceof Error) return err.message;
  return "request failed";
}
