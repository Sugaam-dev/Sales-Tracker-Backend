# CRM Authentication Service (Module A)

A production-ready Authentication Service built using **Go**, **Gin**, **PGX**, and **Supabase PostgreSQL**.

This project implements **Module A (Authentication)** of the CRM system and follows **Clean Architecture**, making it easy to extend with future authentication modules.

---

# Features

## Implemented (Module A)

- User Login
- JWT Authentication
- Refresh Token Rotation
- First-Time Login Flow
- MFA Trigger (OTP Generation)
- Login Rate Limiting
- Password Hashing using bcrypt
- Refresh Token Hashing using SHA-256
- Shared Supabase PostgreSQL Database
- PGX Connection Pool (pgxpool)
- Automatic Startup Database Migration
- Structured Logging (slog)
- Repository Pattern
- Service Layer
- Standardized API Responses

---

# Planned Features

- User Registration
- Password Reset
- Email Verification
- Mobile Verification
- MFA Verification
- Google Login
- Microsoft Azure SSO
- Role-Based Authorization
- Redis Rate Limiting
- Email Service
- SMS Service
- Audit Logging

---

# Technology Stack

| Technology | Version |
|------------|---------|
| Go | 1.25.0 |
| Gin | Latest |
| PGX v5 | Latest |
| pgxpool | Latest |
| Supabase PostgreSQL | Latest |
| JWT | golang-jwt |
| UUID | google/uuid |
| bcrypt | golang.org/x/crypto |
| slog | Go Standard Library |
| Postman | API Testing |

---

# Project Structure

```text
crm-auth-service/

│── main.go
│── go.mod
│── go.sum
│── README.md
│── .env.example
│── .gitignore

├── conf/
├── controllers/
├── helpers/
├── middleware/
├── models/
├── repository/
├── routes/
├── services/
├── scripts/
```

---

# Prerequisites

Install the following before running the project.

- Go 1.25+
- Git
- VS Code (Recommended)
- Access to the shared Supabase project

---

# Clone Repository

```bash
git clone <repository-url>

cd crm-auth-service
```

---

# Install Dependencies

```bash
go mod tidy
```

---

# Environment Setup

Copy

```text
.env.example
```

to

```text
.env
```

Fill the database credentials provided by the project administrator.

Example

```env
APP_ENV=development
SERVER_PORT=8080

DB_HOST=aws-0-ap-southeast-2.pooler.supabase.com
DB_PORT=5432
DB_USER=postgres.xxxxxxxxx
DB_PASSWORD=your-password
DB_NAME=postgres
DB_SSLMODE=require

JWT_SECRET=your-secret-key
```

> Never commit `.env` to GitHub.

---

# Shared Database

This project uses a **shared Supabase PostgreSQL database**.

All developers connect to the same database.

No developer needs to

- Install PostgreSQL
- Create a local database
- Restore SQL dumps

The backend automatically

- Connects to Supabase
- Creates required tables (if missing)
- Runs startup migrations

---

# Running the Project

```bash
go run main.go
```

Server

```
http://localhost:8080
```

---

# Creating Test Users

Default users can be created using

```bash
go run scripts/seed.go
```

The script creates sample users with hashed passwords.

---

# API Endpoints

## Health

```
GET /health
```

---

## Login

```
POST /api/v1/auth/login
```

Example

```json
{
    "identifier":"admin@company.com",
    "password":"Welcome@123"
}
```

---

## Refresh Token

```
POST /api/v1/auth/refresh
```

Example

```json
{
    "refresh_token":"your_refresh_token"
}
```

---

# Authentication Flow

```text
Client
    │
Login Request
    │
Rate Limiter
    │
Find User
    │
Verify Password
    │
First Login Check
    │
MFA Check
    │
Generate Access Token
    │
Generate Refresh Token
    │
Store Refresh Token
    │
Return Response
```

---

# Database Tables

The application manages the following tables.

- users
- refresh_tokens
- mfa_otps

---

# Logging

Logging is implemented using

```
log/slog
```

---

# Security

Implemented security features

- bcrypt Password Hashing
- SHA-256 Refresh Token Hashing
- JWT Authentication
- UUID Primary Keys
- Generic Authentication Errors
- Refresh Token Rotation
- Login Rate Limiting
- Repository Pattern
- Service Layer
- Clean Architecture

---

# Testing APIs

You can test the APIs using

- Postman
- VS Code REST Client

---

# Team Setup

Every developer should

1. Clone the repository
2. Copy `.env.example` → `.env`
3. Configure Supabase credentials
4. Run

```bash
go mod tidy

go run main.go
```

Everyone connects to the same shared Supabase PostgreSQL database.

---

# Git Guidelines

Commit

- Source Code
- go.mod
- go.sum
- README.md
- .env.example
- .gitignore

Never commit

- .env
- Database Passwords
- JWT Secrets
- Build Files
- Executables

---

# Future Roadmap

- User Registration
- Password Reset
- MFA Verification
- Email Verification
- Google Authentication
- Azure SSO
- Redis Integration
- Email Service
- SMS Service
- Audit Logging

---

# Author

**CRM Authentication Service – Module A**

Developed using

- Go
- Gin
- PGX
- pgxpool
- Supabase PostgreSQL
- JWT Authentication