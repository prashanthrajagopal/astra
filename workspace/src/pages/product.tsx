import { Product } from '../types';

const Product = ({ product }: { product: Product }) => {
  return (
    <div className="max-w-md mx-auto p-4">
      <h1 className="text-3xl font-bold">{product.name}</h1>
      <img src={product.image} alt={product.name} className="w-full h-64 object-cover" />
      <p className="text-gray-500">{product.description}</p>
      <p className="text-indigo-500 font-bold">${product.price}</p>
      <p className="text-gray-500">{product.category}</p>
      <p className="text-gray-500">{product.rating}</p>
      <p className="text-gray-500">{product.inStock ? 'In Stock' : 'Out of Stock'}</p>
      <button
        className="bg-indigo-500 p-2 rounded"
        onClick={() => {
          // Add to cart logic here
        }}
      >
        Add to Cart
      </button>
    </div>
  );
};

export default Product;