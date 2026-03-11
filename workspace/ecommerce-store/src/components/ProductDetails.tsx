import { useState } from 'react';

const ProductDetails = () => {
  const productId = process.env.NEXT_ID;

  const [product, setProduct] = useState({
    name: `Product ${productId}`,
    description: `This is a description of product ${productId}`,
    price: 9.99,
    rating: 4.5,
    stockStatus: 'In stock',
  });

  return (
    <div className="flex flex-col justify-center">
      <h2 className="text-3xl font-bold">{product.name}</h2>
      <p className="text-gray-600">{product.description}</p>
      <p className="text-lg font-bold">
        ${product.price.toFixed(2)}
      </p>
      <p className="text-lg font-bold">
        Rating: {product.rating}/5
      </p>
      <p className="text-lg font-bold">
        Stock Status: {product.stockStatus}
      </p>
    </div>
  );
};

export default ProductDetails;