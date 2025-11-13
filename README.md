# **Simple Container**

**Simple Container (`sc`)** is a **cloud-agnostic deployment tool** designed to simplify **microservices deployment, 
infrastructure provisioning, and CI/CD automation**. 

It allows **developers to deploy services effortlessly**, while **DevOps teams manage infrastructure centrally**, 
ensuring **scalability, security, and reliability across multi-cloud environments**.

---

## **Features**

1. **Infrastructure as Code (IaC) Simplified** – Define infrastructure with **lightweight YAML configurations**, no need for Terraform or Pulumi.
2. **Effortless Microservice Deployment** – Deploy services with **one command**, using familiar tools like `Dockerfile` and `docker-compose.yaml`.
3. **Built-in Secrets Management** – Securely store and inject credentials using **AWS Secrets Manager, GCP Secret Manager, or Kubernetes Secrets**.
4. **Multi-Cloud & Kubernetes Support** – Seamlessly deploy to **AWS ECS, Google Cloud Run, Kubernetes clusters**, and more.
5. **Built-in CI/CD & Automated Rollbacks** – Deploy, monitor, and rollback services **without external CI/CD pipelines**.
6. **Comprehensive CloudWatch Monitoring** – Built-in alerts for ECS services and Application Load Balancers with **Email, Slack, Discord, and Telegram** notifications.

---

## **Installation**

To install `sc`, run:
```sh
curl -s "https://dist.simple-container.com/sc.sh" | bash
```
Verify installation:
```sh
sc --version
```

---

## **Getting Started**

1. **Initialize a project**
   ```sh
   sc init
   ```

2. **Configure infrastructure (for DevOps)**
    - Define cloud resources in `.sc/stacks/devops/server.yaml`.

3. **Define a microservice (for developers)**
    - Add a `client.yaml` file in `.sc/stacks/<service-name>/client.yaml`.

4. **Deploy a service**
   ```sh
   sc deploy -s myservice -e staging
   ```

---

## **Documentation**

Check out the [full documentation](https://docs.simple-container.com) for detailed guides, examples, and best practices.

---

## **Contributing**

We welcome contributions! Please see our [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to get involved.

---

## **License**

Simple Container is open-source and licensed under the [MIT License](LICENSE).

---

## **Community & Support**

1. **Feature Requests & Issues** – [GitHub Issues](https://github.com/simple-container-com/api/issues)
2. **Discussions & Updates** – [Join our Discord](https://discord.gg/simple-container)
3. **Enterprise Support** – Contact us at **support@simple-container.com**
4. **News and announcements** - [Follow us on X](https://x.com/simp_container)

Start deploying microservices effortlessly with **Simple Container** today!