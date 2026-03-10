import { createContext, useContext, useState } from 'react';

interface CartState {
  items: any[];
  total: number;
  count: number;
}

const CartContext = createContext<CartState | undefined>(undefined);

const CartStateProvider = ({ children }: { children: React.ReactNode }) => {
  const [cartState, setCartState] = useState<CartState>({
    items: [],
    total: 0,
    count: 0,
  });

  const addToCart = (item: any) => {
    setCartState((prev) => ({
      ...prev,
      items: [...prev.items, item],
      count: prev.count + 1,
      total: prev.total + item.price,
    }));
  };

  const removeFromCart = (item: any) => {
    setCartState((prev) => {
      const newItems = prev.items.filter((i) => i !== item);
      return {
        ...prev,
        items: newItems,
        count: prev.count - 1,
        total: prev.total - item.price,
      };
    });
  };

  const updateQuantity = (item: any, quantity: number) => {
    setCartState((prev) => {
      const newItems = prev.items.map((i) => (i.id === item.id ? { ...i, quantity } : i));
      return {
        ...prev,
        items: newItems,
        count: quantity,
        total: newItems.reduce((acc, item) => acc + item.quantity * item.price, 0),
      };
    });
  };

  const clearCart = () => {
    setCartState({ items: [], total: 0, count: 0 });
  };

  return (
    <CartContext.Provider value={{ ...cartState, addToCart, removeFromCart, updateQuantity, clearCart }}>
      {children}
    </CartContext.Provider>
  );
};

export { CartContext, CartStateProvider };