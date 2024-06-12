---
title: Index page for Simple Container Docs
description: 'simple-container.com offers high-level primitives for quick and easy set-up of integration and delivery pipelines' # A short summary for search engines to display, max 120 chars
platform: platform
product: simple-container
category: devguide
subcategory: index
date: '2024-06-12'
---

# Simple Container

Unlike other products whose main focus usually is to provide fine-grained configuration options for either CI or CD 
aspect of software delivery, Simple Container offers high-level primitives for quick and easy set-up of integration 
and delivery pipelines for microservice applications. Simple Container reflects simplicity of use and 
container-native nature of the provided DevOps practices.

## Why do I need Simple-Container.com?

Simple Container allows companies to easily set up their microservices infrastructure with a few simple steps.
With just a few commands they should easily get ready-to-use CI/CD pipelines that can build and deploy both a single
microservice and scale up to hundreds of them.

If you're already familiar with tools like [Terraform](https://www.terraform.io/) or [Pulumi](https://www.pulumi.com/), 
you should know that development and maintenance of [Infrastructure as Code](https://en.wikipedia.org/wiki/Infrastructure_as_code) 
using such tools is not always an easy task, especially when it comes to configuration of things like secrets, scaling, observability etc.

Usually, developers and DevOps have to work on the same project, and it requires collaboration and causes delays 
and misunderstanding. Simple Container solves the problem by providing a tool set developers are most familiar with, but
at the same time giving the easy higher level abstractions allowing to map their local development environment to the 
chosen cloud provider primitives.

While Simple Container is not a CI/CD tool by itself, it is mainly focused on the build and deploy lifecycle of a
service, trying to make the following operations easier and more streamlined:

* Simplicity of CI/CD set-up which starts from a single and concise service manifest Docker Compose-like files
* Higher level of Infrastructure as Code approach allowing to focus only on the most important aspects of deployments
* Ability to easily and quickly scale infrastructure from a single deployment to thousands of various resources in the
cloud or hosted Kubernetes clusters
* Setting up DevOps pipelines and infrastructure must be a self-service operation for any developer without tons of special
  knowledge required
* Configuration of cloud-native primitives, such as 

Please read [Motivation](/motivation/) to understand where Simple Container fits in the development process.

## Getting started

Read [howto install simple-container](/howto/install/) and [main use-cases](/howto/) to get started.

## Questions/Issues?

If you have any issues or questions related to Simple-Container.com, please reach out at [support@simple-container.com](mailto:support@simple-container.com).