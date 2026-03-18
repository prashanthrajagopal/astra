app/db.py
import sqlite3

class Database:
    def __init__(self):
        self.conn = sqlite3.connect('test.db')
        self.conn.row_factory = sqlite3.Row

    def query(self, query, *args):
        cur = self.conn.cursor()
        cur.execute(query, *args)
        return [dict(row) for row in cur.fetchall()]

    def close(self):
        self.conn.close()
