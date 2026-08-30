const endpoint = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

export type Money = { amountCents: number; currency: string };

export type Variant = {
  id: string;
  sku: string;
  size: string;
  finish: string;
  unitPrice: Money;
  available: number;
};

export type Product = {
  id: string;
  name: string;
  description: string;
  variants: Variant[];
};

export type OrderStatus = "PENDING" | "PAID" | "PRINTING" | "SHIPPED" | "CANCELLED";

export type Order = {
  id: string;
  status: OrderStatus;
  email: string;
  total: Money;
  createdAt: string;
  lines: { quantity: number; unitPrice: Money; variant: Variant }[];
};

type ErrorBody = { detail?: { code?: string; message?: string } | string };

export class ApiError extends Error {
  code?: string;
  constructor(message: string, code?: string) {
    super(message);
    this.code = code;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${endpoint}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  if (res.status === 404) {
    return null as T;
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as ErrorBody;
    const detail = body.detail;
    if (detail && typeof detail === "object") {
      throw new ApiError(detail.message ?? `HTTP ${res.status}`, detail.code);
    }
    throw new ApiError(typeof detail === "string" ? detail : `HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

export function fetchProducts() {
  return request<Product[]>("/products");
}

export function checkout(input: {
  idempotencyKey: string;
  email: string;
  lines: { variantId: string; quantity: number }[];
}) {
  return request<Order>("/checkout", { method: "POST", body: JSON.stringify(input) });
}

export function fetchOrder(id: string) {
  return request<Order | null>(`/orders/${id}`);
}

export function markPaid(id: string) {
  return request<Order>(`/orders/${id}/paid`, { method: "POST" });
}

export function startPrint(id: string) {
  return request<Order>(`/orders/${id}/print`, { method: "POST" });
}

export function markShipped(id: string) {
  return request<Order>(`/orders/${id}/ship`, { method: "POST" });
}

export function cancelOrder(id: string) {
  return request<Order>(`/orders/${id}/cancel`, { method: "POST" });
}
