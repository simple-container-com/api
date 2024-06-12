---
title: Main use-cases for Simple-Container.com
description: What Simple Container can do for you?
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# Main use-cases for Simple Container

## Separation of DevOps and Developer parties

Simple Container allows DevOps of the company to easily set up the basics of the infrastructure using the chosen
cloud provider (be it an AWS or GCP cloud or even a hosted Kubernetes cluster), prepare main secrets and account 
configurations ahead of time.
DevOps should simply prepare a basic configuration of their resources and then invoke a single command `sc provision`
which will take care of the initial provision steps, and will create all resources a company needs.

For developers Simple Container provides the high-level abstraction allowing to easily set up their CI/CD pipeline and 
deploy their services into the provisioned cloud. With Simple Container adding a new microservice a company needs takes
a couple of very simple steps making the whole process a self-service operation without much help needed from DevOps.
