src/types/CartItem.ts
interface CartItem {
  product: {
    id: string;
    name: string;
    price: number;
  };
  quantity: number;
}
