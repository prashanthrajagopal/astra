import { Html, Head, Main, NextScript } from 'next';
import { ReactText } from 'react';

function Document({ children }: { children: ReactText }) {
  return (
    <Html>
      <Head />
      <body>
        <Main>{children}</Main>
        <NextScript />
      </body>
    </Html>
  );
}

export default Document;