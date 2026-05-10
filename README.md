# vedx-backend

A simple backend service for user authentication, role-based access, and course management.

## API

### Register user

Endpoint: `POST /auth/register`

Required JSON body:

```json
{
  "email": "admin@example.com",
  "password": "Secret@123",
  "first_name": "Admin",
  "last_name": "User",
  "role": "super_admin"
}
```

- `email`: required, valid email
- `password`: required, minimum 8 characters
- `first_name`: required
- `last_name`: required
- `role`: required, one of `student`, `mentor`, `employee`, `team_lead`, `super_admin`

### Super admin

To create a super admin, set `role` to `super_admin` in the register payload.

Optional fields:
- `phone`
- `date_of_birth`
