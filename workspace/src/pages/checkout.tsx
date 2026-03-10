import { useCart } from '../context/CartContext';

const Checkout = () => {
  const { cartItems, cartCount } = useCart();

  return (
    <div className="max-w-md mx-auto p-4">
      <h1 className="text-3xl font-bold">Checkout</h1>
      <p className="text-lg font-bold">
        You have {cartCount} items in your cart. Total: ${cartTotal()}
      </p>
      <form>
        <label>
          Shipping Information:
          <input type="text" placeholder="Name" />
          <input type="email" placeholder="Email" />
          <input type="text" placeholder="Address" />
          <input type="text" placeholder="City" />
          <input type="text" placeholder="Zip" />
        </label>
      </form>
      <button
        className="bg-indigo-500 p-2 rounded"
        onClick={() => {
          // Store to localStorage logic here
        }}
      >
        Place Order
      </button>
    </div>
  );
};

export default Checkout;