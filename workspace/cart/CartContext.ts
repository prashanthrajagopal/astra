import { createContext, useState, useEffect } from 'react';

interface CartItem {
  product: Product;
  quantity: number;
}

interface CartState {
  cartItems: CartItem[];
  cartTotal: number;
  cartCount: number;
}

const CartContext = createContext<CartState | null>(null);

const CartProvider = ({ children }) => {
  const [cartItems, setCartItems] = useState<CartItem[]>([]);
  const [cartTotal, setCartTotal] = useState(0);
  const [cartCount, setCartCount] = useState(0);

  useEffect(() => {
    setCartTotal(cartItems.reduce((acc, item) => acc + item.product.price * item.quantity, 0));
    setCartCount(cartItems.length);
  }, [cartItems]);

  const addToCart = (product: Product, quantity: number) => {
    // Add product to cart
  };

  const removeFromCart = (product: Product) => {
    // Remove product from cart
  };

  const updateQuantity = (product: Product, quantity: number) => {
    // Update product quantity in cart
  };

  const clearCart = () => {
    // Clear cart
  };

  const cartTotal = () => {
    return cartTotal;
  };

  const cartCount = () => {
    return cartCount;
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
        clearCart
      }}
    >
      {children}
    </CartContext.Provider>
  );
};

export { CartProvider, CartContext };