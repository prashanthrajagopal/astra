import { createTheme } from '@mui/material/styles';
import { blue, green, indigo, red, teal } from '@mui/material/colors';

const theme = createTheme({
  palette: {
    primary: {
      main: blue[500],
    },
    secondary: {
      main: green[500],
    },
    info: {
      main: teal[500],
    },
    error: {
      main: red[500],
    },
    background: {
      default: indigo[50],
    },
  },
});

export default theme;