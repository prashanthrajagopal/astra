import { createContext, useState, useContext } from 'react';
import { useCart } from './useCart';

type CartItem = {
  id: string;
  quantity: number;
};

interface CartContextProps {
  cartItems: CartItem[];
  cartTotal: number;
  cartCount: number;
  addToCart: (item: CartItem) => void;
  removeFromCart: (id: string) => void;
  updateQuantity: (id: string, quantity: number) => void;
  clearCart: () => void;
}

const CartContext = createContext<CartContextProps>({
  cartItems: [],
  cartTotal: 0,
  cartCount: 0,
  addToCart: () => {},
  removeFromCart: () => {},
  updateQuantity: () => {},
  clearCart: () => {},
});

const CartProvider: React.FC = ({ children }) => {
  const [cartItems, setCartItems] = useState<CartItem[]>([]);
  const [cartTotal, setCartTotal] = useState(0);
  const [cartCount, setCartCount] = useState(0);

  const addToCart = (item: CartItem) => {
    setCartItems((prevItems) => [...prevItems, item]);
    setCartTotal((prevTotal) => prevTotal + item.quantity);
    setCartCount((prevCount) => prevCount + 1);
  };

  const removeFromCart = (id: string) => {
    setCartItems((prevItems) =>
      prevItems.filter((item) => item.id !== id)
    );
    setCartTotal((prevTotal) =>
      prevTotal - cartItems.find((item) => item.id === id)?.quantity || 0
    );
    setCartCount((prevCount) => prevCount - 1);
  };

  const updateQuantity = (id: string, quantity: number) => {
    setCartItems((prevItems) =>
      prevItems.map((item) => (item.id === id ? { ...item, quantity } : item))
    );
    setCartTotal((prevTotal) =>
      prevTotal - cartItems.find((item) => item.id === id)?.quantity + quantity
    );
  };

  const clearCart = () => {
    setCartItems([]);
    setCartTotal(0);
    setCartCount(0);
  };

  return (
    <CartContext.Provider
      value={{
        cartItems,
        cartTotal,
        cartCount,
        addToCart,
        removeFromCart,
        updateQuantity,
        clearCart,
      }}
    >
      {children}
    </CartContext.Provider>
  );
};

export { CartProvider, CartContext };