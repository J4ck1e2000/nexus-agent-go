/** Auto-extracted from web/login/login-app.js (keeps its function values verbatim). */
export const loginMessages = {
    en: {
        page: {
            titleLogin: 'Nexus Monitor - Sign In',
            titleRegister: 'Nexus Monitor - Register',
        },
        brand: {
            name: 'Nexus Monitor',
            tagline: 'Cluster Access',
        },
        language: {
            label: 'Language',
            switchAriaLabel: 'Switch language',
            options: {
                en: 'English',
                zh: '中文',
            },
        },
        mode: {
            login: 'Sign in',
            register: 'Register',
        },
        panel: {
            loginTitle: 'Welcome back',
            loginSubtitle: 'Sign in to access your cluster dashboard',
            registerTitle: 'Create account',
            registerSubtitle: 'Register to access your cluster dashboard',
        },
        form: {
            usernameLabel: 'Username',
            passwordLabel: 'Password',
            confirmPasswordLabel: 'Confirm Password',
            usernamePlaceholder: 'Enter your username',
            passwordPlaceholderLogin: 'Enter your password',
            passwordPlaceholderRegister: 'At least 6 characters',
            confirmPasswordPlaceholder: 'Re-enter your password',
        },
        submit: {
            login: 'Sign in',
            register: 'Create account',
            loggingIn: 'Signing in...',
            registering: 'Creating account...',
        },
        footer: {
            note: 'Monitor distributed GPU nodes with a calm, real-time dashboard.',
        },
        validation: {
            usernameRequired: 'Username is required.',
            passwordRequired: 'Password is required.',
            confirmPasswordRequired: 'Please confirm your password.',
            passwordTooShort: 'Password must be at least 6 characters.',
            passwordMismatch: 'Password and confirmation do not match.',
        },
        feedback: {
            loginEmptyToken: 'Login failed: empty token.',
            networkLogin: 'Unable to connect to server. Please retry.',
            networkRegister: 'Unable to connect to server. Please retry registration.',
            registrationSuccess: 'Registration successful. Please sign in.',
        },
        errors: {
            login: {
                invalidCredentials: 'Invalid username or password.',
                generic: 'Login failed. Please try again.',
                withCode: ({ errorCode }: { errorCode: string }) => `Login failed: ${errorCode}`,
            },
            register: {
                userAlreadyExists: 'Username already exists.',
                invalidUsername: 'Username must be 3-64 characters and only contain letters, numbers, dot (.), underscore (_) or hyphen (-).',
                passwordTooShort: 'Password must be at least 6 characters.',
                invalidInput: 'Please check your username and password format.',
                generic: 'Registration failed. Please try again.',
                withCode: ({ errorCode }: { errorCode: string }) => `Registration failed: ${errorCode}`,
            },
        },
    },
    zh: {
        page: {
            titleLogin: 'Nexus Monitor - 登录',
            titleRegister: 'Nexus Monitor - 注册',
        },
        brand: {
            name: 'Nexus Monitor',
            tagline: '集群访问',
        },
        language: {
            label: '语言',
            switchAriaLabel: '切换语言',
            options: {
                en: 'English',
                zh: '中文',
            },
        },
        mode: {
            login: '登录',
            register: '注册',
        },
        panel: {
            loginTitle: '欢迎回来',
            loginSubtitle: '登录后即可访问你的集群仪表盘',
            registerTitle: '创建账号',
            registerSubtitle: '注册后即可访问你的集群仪表盘',
        },
        form: {
            usernameLabel: '用户名',
            passwordLabel: '密码',
            confirmPasswordLabel: '确认密码',
            usernamePlaceholder: '请输入用户名',
            passwordPlaceholderLogin: '请输入密码',
            passwordPlaceholderRegister: '至少 6 个字符',
            confirmPasswordPlaceholder: '请再次输入密码',
        },
        submit: {
            login: '登录',
            register: '创建账号',
            loggingIn: '登录中...',
            registering: '创建中...',
        },
        footer: {
            note: '使用沉稳、实时的面板监控分布式 GPU 节点。',
        },
        validation: {
            usernameRequired: '用户名不能为空。',
            passwordRequired: '密码不能为空。',
            confirmPasswordRequired: '请确认密码。',
            passwordTooShort: '密码至少需要 6 个字符。',
            passwordMismatch: '密码与确认密码不一致。',
        },
        feedback: {
            loginEmptyToken: '登录失败：令牌为空。',
            networkLogin: '无法连接服务器，请稍后重试。',
            networkRegister: '无法连接服务器，请稍后重试注册。',
            registrationSuccess: '注册成功，请登录。',
        },
        errors: {
            login: {
                invalidCredentials: '用户名或密码错误。',
                generic: '登录失败，请重试。',
                withCode: ({ errorCode }: { errorCode: string }) => `登录失败：${errorCode}`,
            },
            register: {
                userAlreadyExists: '用户名已存在。',
                invalidUsername: '用户名长度需为 3-64 个字符，且只能包含字母、数字、点(.)、下划线(_)或连字符(-)。',
                passwordTooShort: '密码至少需要 6 个字符。',
                invalidInput: '请检查用户名和密码格式。',
                generic: '注册失败，请重试。',
                withCode: ({ errorCode }: { errorCode: string }) => `注册失败：${errorCode}`,
            },
        },
    },
};
