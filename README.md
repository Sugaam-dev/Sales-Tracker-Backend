# CRM Authentication Service (Module A)

A production-ready Authentication Service built using **Go**, **Gin**, **GORM**, and **Supabase PostgreSQL**.

This project implements **Module A (Authentication)** of the CRM system and is designed with a modular architecture so future authentication features can be added without major code changes.
---

# Features

## Implemented (Module A)
- User Login
- JWT Authentication
- Refresh Token Rotation
- First-Time Login Flow
- MFA Trigger (OTP Generation)
- Login Rate Limiting
- Password Hash Verification (bcrypt)
- Refresh Token Hashing (SHA-256)
- PostgreSQL Integration (Supabase)
- Automatic Database Migration (GORM AutoMigrate)
- Structured Logging using slog
- Clean Architecture
- Repository Pattern
- Service Layer
- Standardized API Responses
---

# Planned Future Modules
The architecture is designed to support future CRM authentication features.
- User Registration
- User Onboarding
- Password Reset
- Email Verification
- Mobile Verification
- MFA Verification
- Microsoft Entra ID (Azure AD) SSO
- Role Based Authorization
- Redis Rate Limiting
- Email Service
- SMS Service
---

# Technology Stack

| Technology | Version |
|------------|---------|
| Go | 1.22+ |
| Gin | Latest |
| GORM | Latest |
| Supabase PostgreSQL | Latest |
| JWT | golang-jwt |
| UUID | google/uuid |
| bcrypt | golang.org/x/crypto |
| slog | Go Standard Library |
| Postman | API Testing |
---

# Project Structure
```
crm-auth-service/
│
├── main.go
├── go.mod
├── go.sum
├── .gitignore
├── .env.example
├── README.md
├── api_tests.http
│
├── conf/
├── controllers/
├── helpers/
├── middleware/
├── models/
├── repository/
├── routes/
└── services/
```
---

# Prerequisites
Before running the project, install:
- Go 1.22 or higher
- Git
- VS Code (Recommended)
- Access to the shared Supabase project
---

# Clone the Repository
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

# Environment Variables
Copy the template file.
```bash
cp .env.example .env
```

Fill the `.env` file using the shared Supabase credentials provided by the project administrator.

Example:
```env
APP_NAME=Sales Tracker Portal
APP_ENV=development
SERVER_PORT=8080

DB_HOST=your-session-pooler-host
DB_PORT=5432
DB_USER=postgres.your-project-reference
DB_PASSWORD=your-password
DB_NAME=postgres
DB_SSLMODE=require

JWT_SECRET=your-secret-key
```

> **Important**
> Never commit the `.env` file to GitHub.
> Only `.env.example` should be committed.
---

# Shared Database
This project uses a **shared Supabase PostgreSQL database**.
All developers connect to the same database.

No developer needs to:
- Install PostgreSQL
- Create a local database
- Import SQL scripts
- Manually create tables

On application startup, the backend automatically:
- Connects to Supabase
- Runs GORM AutoMigrate
- Creates missing tables
- Creates indexes and constraints
---

# Running the Project
Start the application.
```bash
go run main.go
```
If everything is configured correctly, the server starts on:
```
http://localhost:8080
```
---

# API Endpoints
## Health Check
```
GET /health
```
---

## Login
```
POST /api/v1/auth/login
```
Request Body
```json
{
    "identifier": "admin@example.com",
    "password": "Password123!"
}
```
---

## Refresh Token
```
POST /api/v1/auth/refresh
```
Request Body
```json
{
    "refresh_token": "<refresh_token>"
}
```
---

# Authentication Flow
```
Client
↓
Login Request
↓
Rate Limiter
↓
Find User
↓
Verify Password
↓
First Login Check
↓
MFA Check
↓
Generate JWT
↓
Generate Refresh Token
↓
Store Refresh Token
↓
Return Response
```
---

# Database
The following tables are automatically created using GORM AutoMigrate.
- users
- refresh_tokens
- mfa_otps

No SQL scripts are required.
---

# Logging
Structured logging is implemented using Go's standard logging package.
```
log/slog
```
---

# Security
The authentication module follows secure development practices.
- bcrypt Password Hashing
- SHA-256 Refresh Token Hashing
- JWT Authentication
- UUID Primary Keys
- Generic Authentication Errors
- Secure OTP Generation
- Rate Limiting
- Refresh Token Rotation
- Repository Pattern
- Service Layer Separation
---

# Testing APIs
The APIs can be tested using:
- Postman
- api_tests.http (VS Code REST Client)
---

# Team Setup
Every developer follows the same setup process.
1. Clone the repository.
2. Copy `.env.example` to `.env`.
3. Obtain the shared Supabase credentials from the project administrator.
4. Update the `.env` file.
5. Run:

```bash
go mod tidy
go run main.go
```

All developers will connect to the same shared Supabase PostgreSQL database.
---

# Git Guidelines
Commit to GitHub:
- Source Code
- go.mod
- go.sum
- README.md
- .env.example
- .gitignore

Do NOT commit:
- .env
- Database Passwords
- JWT Secrets
- Build Files
- Executables
- Temporary Files
---

# Future Enhancements
- User Registration
- Password Reset
- Email Verification
- Mobile Verification
- MFA Verification
- Microsoft Entra ID (Azure AD) SSO
- Redis Integration
- Email Service
- SMS Service
- Audit Logging
---

# Author
**CRM Authentication Service – Module A**

Developed using:
- Go
- Gin
- GORM
- Supabase PostgreSQL
- JWT Authentication