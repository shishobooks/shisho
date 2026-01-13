import { Loader2 } from "lucide-react";
import { useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import Logo from "@/components/library/Logo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Toaster } from "@/components/ui/sonner";
import { useAuth } from "@/hooks/useAuth";

const Login = () => {
  const navigate = useNavigate();
  const { isAuthenticated, needsSetup, isLoading: authLoading } = useAuth();
  const { login } = useAuth();

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!username.trim() || !password) {
      toast.error("Please enter both username and password");
      return;
    }

    setIsLoading(true);
    try {
      await login(username, password);
      navigate("/");
    } catch (error) {
      let msg = "Login failed. Please check your credentials.";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    } finally {
      setIsLoading(false);
    }
  };

  if (authLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Redirect to setup if needed
  if (needsSetup) {
    return <Navigate replace to="/setup" />;
  }

  // Redirect if already authenticated
  if (isAuthenticated) {
    return <Navigate replace to="/" />;
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <Toaster richColors />
      <div className="w-full max-w-md p-8">
        <div className="flex flex-col items-center mb-8">
          <Logo className="mb-2" size="lg" />
          <p className="text-muted-foreground text-center">
            Sign in to access your library
          </p>
        </div>

        <form className="space-y-6" onSubmit={handleSubmit}>
          <div className="space-y-2">
            <Label htmlFor="username">Username</Label>
            <Input
              autoComplete="username"
              autoFocus
              id="username"
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Enter your username"
              type="text"
              value={username}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              autoComplete="current-password"
              id="password"
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter your password"
              type="password"
              value={password}
            />
          </div>

          <Button className="w-full" disabled={isLoading} type="submit">
            {isLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Signing in...
              </>
            ) : (
              "Sign in"
            )}
          </Button>
        </form>
      </div>
    </div>
  );
};

export default Login;
