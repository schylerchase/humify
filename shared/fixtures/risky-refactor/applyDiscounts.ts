type CartItem = {
  sku: string;
  quantity: number;
  unitPrice: number;
  category: string;
};

type Discount = {
  code: string;
  percent: number;
  category?: string;
  minimumSpend?: number;
};

export function applyDiscounts(items: CartItem[], discounts: Discount[]) {
  let subtotal = 0;
  let discountTotal = 0;

  for (const item of items) {
    subtotal += item.unitPrice * item.quantity;
  }

  for (const discount of discounts) {
    let eligibleTotal = 0;

    for (const item of items) {
      if (discount.category && item.category !== discount.category) {
        continue;
      }

      eligibleTotal += item.unitPrice * item.quantity;
    }

    if (discount.minimumSpend && eligibleTotal < discount.minimumSpend) {
      continue;
    }

    discountTotal += eligibleTotal * discount.percent;
  }

  return {
    subtotal,
    discountTotal,
    total: subtotal - discountTotal,
  };
}

