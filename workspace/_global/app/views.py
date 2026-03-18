app/views.py
from app import app
from app.models import User

@app.route("/")
def index():
    users = User.query.all()
    return {"users": [{"name": u.name, "email": u.email} for u in users]}

if __name__ == "__main__":
    app.run(debug=True)
