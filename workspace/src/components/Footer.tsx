const Footer = () => {
  return (
    <footer className="bg-gray-800 p-4 shadow-md">
      <p className="text-gray-300 text-center">
        &copy; {new Date().getFullYear()} My App
      </p>
    </footer>
  );
};

export default Footer;