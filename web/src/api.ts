const endpoint = import.meta.env.VITE_API_URL ?? "http://localhost:8080/graphql";

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

type GraphQLError = { message: string; extensions?: { code?: string } };

type GraphQLResult<T> = { data?: T; errors?: GraphQLError[] };

export class ApiError extends Error {
  code?: string;
  constructor(message: string, code?: string) {
    super(message);
    this.code = code;
  }
}

async function gql<T>(query: string, variables?: Record<string, unknown>): Promise<T> {
  const res = await fetch(endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, variables }),
  });
  if (!res.ok) {
    throw new ApiError(`HTTP ${res.status}`);
  }
  const body = (await res.json()) as GraphQLResult<T>;
  if (body.errors?.length) {
    throw new ApiError(body.errors[0].message, body.errors[0].extensions?.code);
  }
  if (!body.data) {
    throw new ApiError("empty GraphQL response");
  }
  return body.data;
}

const ORDER_FIELDS = `
  id status email createdAt
  total { amountCents currency }
  lines {
    quantity
    unitPrice { amountCents currency }
    variant { id sku size finish available unitPrice { amountCents currency } }
  }
`;

export function fetchProducts() {
  return gql<{ products: Product[] }>(`
    query {
      products {
        id name description
        variants {
          id sku size finish available
          unitPrice { amountCents currency }
        }
      }
    }
  `).then((d) => d.products);
}

export function checkout(input: {
  idempotencyKey: string;
  email: string;
  lines: { variantId: string; quantity: number }[];
}) {
  return gql<{ checkout: Order }>(
    `mutation ($input: CheckoutInput!) { checkout(input: $input) { ${ORDER_FIELDS} } }`,
    { input },
  ).then((d) => d.checkout);
}

export function fetchOrder(id: string) {
  return gql<{ order: Order | null }>(
    `query ($id: ID!) { order(id: $id) { ${ORDER_FIELDS} } }`,
    { id },
  ).then((d) => d.order);
}

export function markPaid(id: string) {
  return gql<{ markPaid: Order }>(`mutation ($id: ID!) { markPaid(id: $id) { ${ORDER_FIELDS} } }`, {
    id,
  }).then((d) => d.markPaid);
}

export function startPrint(id: string) {
  return gql<{ startPrint: Order }>(
    `mutation ($id: ID!) { startPrint(id: $id) { ${ORDER_FIELDS} } }`,
    { id },
  ).then((d) => d.startPrint);
}

export function markShipped(id: string) {
  return gql<{ markShipped: Order }>(
    `mutation ($id: ID!) { markShipped(id: $id) { ${ORDER_FIELDS} } }`,
    { id },
  ).then((d) => d.markShipped);
}

export function cancelOrder(id: string) {
  return gql<{ cancelOrder: Order }>(
    `mutation ($id: ID!) { cancelOrder(id: $id) { ${ORDER_FIELDS} } }`,
    { id },
  ).then((d) => d.cancelOrder);
}
